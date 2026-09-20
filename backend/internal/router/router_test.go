package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/config"
	"github.com/blueship581/print-color-calibration-release/backend/internal/database"
	"github.com/blueship581/print-color-calibration-release/backend/internal/router"
	"github.com/gin-gonic/gin"
)

type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

func TestRBACAndImmutableRevisionFlows(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "gb517.db"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, redisClient, logger)
	tokens := map[string]string{}
	for _, role := range []string{"viewer", "operator", "reviewer", "admin"} {
		tokens[role] = loginToken(t, engine, role)
	}

	payload := recordPayload("RD-TEST-001", "测试放行决定")
	if status, _ := perform(t, engine, http.MethodPost, "/api/release", tokens["viewer"], "viewer-create", payload); status != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, want 403", status)
	}
	status, body := perform(t, engine, http.MethodPost, "/api/release", tokens["operator"], "decision-create", payload)
	if status != http.StatusCreated {
		t.Fatalf("operator create decision status = %d body=%s", status, body)
	}
	decision := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	transition := map[string]any{"status": "release", "expectedVersion": decision.Version, "reason": "quality gate accepted"}
	path := "/api/release/" + uintString(decision.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, path, tokens["operator"], "operator-release", transition); status != http.StatusForbidden {
		t.Fatalf("operator release status = %d, want 403", status)
	}
	status, body = perform(t, engine, http.MethodPost, path, tokens["reviewer"], "reviewer-release", transition)
	if status != http.StatusOK {
		t.Fatalf("reviewer release status = %d body=%s", status, body)
	}
	status, body = perform(t, engine, http.MethodGet, "/api/release/"+uintString(decision.ID), tokens["reviewer"], "decision-read", nil)
	detail := decodeData[struct {
		Version   uint `json:"version"`
		Revisions []struct {
			Version   uint   `json:"version"`
			RequestID string `json:"requestId"`
		} `json:"revisions"`
	}](t, body)
	if status != http.StatusOK || detail.Version != 2 || len(detail.Revisions) != 2 || detail.Revisions[0].RequestID != "reviewer-release" {
		t.Fatalf("unexpected decision revision chain: status=%d detail=%+v", status, detail)
	}
	update := recordPayload("ignored", "不得覆盖的决定")
	update["expectedVersion"] = detail.Version
	if status, _ := perform(t, engine, http.MethodPut, "/api/release/"+uintString(decision.ID), tokens["operator"], "locked-update", update); status != http.StatusConflict {
		t.Fatalf("resolved decision update status = %d, want 409", status)
	}
	if status, _ := perform(t, engine, http.MethodDelete, "/api/release/"+uintString(decision.ID), tokens["admin"], "locked-delete", nil); status != http.StatusConflict {
		t.Fatalf("resolved decision delete status = %d, want 409", status)
	}

	runPayload := recordPayload("PR-TEST-001", "测试色彩配置")
	status, body = perform(t, engine, http.MethodPost, "/api/runs", tokens["operator"], "run-create", runPayload)
	run := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	if status != http.StatusCreated {
		t.Fatalf("create run status = %d body=%s", status, body)
	}
	runPath := "/api/runs/" + uintString(run.ID) + "/transition"
	status, _ = perform(t, engine, http.MethodPost, runPath, tokens["operator"], "run-printing", map[string]any{"status": "printing", "expectedVersion": run.Version, "reason": "plates and ink verified"})
	if status != http.StatusOK {
		t.Fatalf("run transition status = %d", status)
	}
	if status, _ := perform(t, engine, http.MethodDelete, "/api/runs/"+uintString(run.ID), tokens["admin"], "locked-run-delete", nil); status != http.StatusConflict {
		t.Fatalf("active run delete status = %d, want 409", status)
	}
	_, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(run.ID), tokens["operator"], "run-read", nil)
	runDetail := decodeData[struct {
		Revisions []struct {
			RequestID string `json:"requestId"`
		} `json:"revisions"`
	}](t, body)
	if len(runDetail.Revisions) != 2 || runDetail.Revisions[0].RequestID != "run-printing" {
		t.Fatalf("unexpected colour configuration revisions: %+v", runDetail.Revisions)
	}

	if status, _ := perform(t, engine, http.MethodGet, "/api/audits", tokens["viewer"], "viewer-audit", nil); status != http.StatusForbidden {
		t.Fatalf("viewer audit status = %d, want 403", status)
	}
	if status, _ := perform(t, engine, http.MethodGet, "/api/audits", tokens["reviewer"], "reviewer-audit", nil); status != http.StatusOK {
		t.Fatalf("reviewer audit status = %d, want 200", status)
	}
}

func testConfig(dsn string) config.Config {
	return config.Config{
		AppName: "print-color-calibration-release", Environment: "test", Port: "0",
		DatabaseDriver: "sqlite", DatabaseDSN: dsn, JWTSecret: "gb517-router-tests-secret",
		TokenTTL: time.Hour, RequestLimit: 1000, StartupTimeout: time.Second,
		ShutdownTimeout: time.Second, ReadHeaderTimeout: time.Second, ReadTimeout: time.Second,
		WriteTimeout: time.Second, IdleTimeout: time.Second,
	}
}

func loginToken(t *testing.T, engine *gin.Engine, username string) string {
	t.Helper()
	status, body := perform(t, engine, http.MethodPost, "/api/auth/login", "", "login-"+username, map[string]any{"username": username, "password": "Admin123!"})
	if status != http.StatusOK {
		t.Fatalf("login %s status = %d body=%s", username, status, body)
	}
	return decodeData[struct {
		Token string `json:"token"`
	}](t, body).Token
}

func recordPayload(code, name string) map[string]any {
	return map[string]any{
		"code": code, "name": name, "description": "router integration test",
		"facility": "测试印刷区", "owner": "operator", "category": "校准",
		"riskLevel": "medium", "metricValue": 2.1, "metricUnit": "dE",
		"effectiveAt": time.Now().UTC().Format(time.RFC3339), "evidence": "spectrophotometer evidence", "relatedCode": "PR-001",
	}
}

func perform(t *testing.T, engine *gin.Engine, method, path, token, requestID string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("X-Request-ID", requestID)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response.Code, response.Body.Bytes()
}

func decodeData[T any](t *testing.T, body []byte) T {
	t.Helper()
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode envelope %s: %v", body, err)
	}
	var value T
	if err := json.Unmarshal(envelope.Data, &value); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	return value
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func driftPayload(code, relatedCode string, metricValue float64) map[string]any {
	return map[string]any{
		"code": code, "name": "漂移门禁测试校样", "description": "drift gate integration test",
		"facility": "测试印刷区", "owner": "operator", "category": "常规",
		"riskLevel": "medium", "metricValue": metricValue, "metricUnit": "dE",
		"effectiveAt": time.Now().UTC().Format(time.RFC3339), "evidence": "spectrophotometer evidence",
		"relatedCode": relatedCode,
	}
}

func proofView(t *testing.T, body []byte) struct {
	ID              uint    `json:"id"`
	Version         uint    `json:"version"`
	Status          string  `json:"status"`
	GateStatus      string  `json:"gateStatus"`
	GateBlockReason string  `json:"gateBlockReason"`
	GateBaseline    float64 `json:"gateBaseline"`
	GateDeviation   float64 `json:"gateDeviation"`
	GateTolerance   float64 `json:"gateTolerance"`
	GateSampleSize  int     `json:"gateSampleSize"`
	Superseded      bool    `json:"superseded"`
} {
	return decodeData[struct {
		ID              uint    `json:"id"`
		Version         uint    `json:"version"`
		Status          string  `json:"status"`
		GateStatus      string  `json:"gateStatus"`
		GateBlockReason string  `json:"gateBlockReason"`
		GateBaseline    float64 `json:"gateBaseline"`
		GateDeviation   float64 `json:"gateDeviation"`
		GateTolerance   float64 `json:"gateTolerance"`
		GateSampleSize  int     `json:"gateSampleSize"`
		Superseded      bool    `json:"superseded"`
	}](t, body)
}

func createProof(t *testing.T, engine *gin.Engine, token, requestID, code, relatedCode string, metricValue float64) []byte {
	t.Helper()
	status, body := perform(t, engine, http.MethodPost, "/api/proofs", token, requestID, driftPayload(code, relatedCode, metricValue))
	if status != http.StatusCreated {
		t.Fatalf("create proof %s status = %d body=%s", code, status, body)
	}
	return body
}

func moveProof(t *testing.T, engine *gin.Engine, token, requestID string, id, version uint, target string, wantStatus int) ([]byte, []byte) {
	t.Helper()
	path := "/api/proofs/" + uintString(id) + "/transition"
	payload := map[string]any{"status": target, "expectedVersion": version, "reason": requestID}
	status, body := perform(t, engine, http.MethodPost, path, token, requestID, payload)
	if status != wantStatus {
		t.Fatalf("transition proof %d -> %s status = %d want %d body=%s", id, target, status, wantStatus, body)
	}
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(body, &envelope)
	return []byte(envelope.Data), []byte(envelope.Message)
}

func TestColorProofDriftGate(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "gb517-drift.db"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, redisClient, logger)
	operator := loginToken(t, engine, "operator")
	reviewer := loginToken(t, engine, "reviewer")
	const related = "REL-DRIFT-1"

	// 同一关联编码+类别下建立 3 份已接受校样，中位数基准为 10.0。
	for index, value := range []float64{10.0, 10.2, 9.8} {
		code := "CP-DRIFT-B" + strconv.Itoa(index+1)
		proof := proofView(t, createProof(t, engine, operator, "baseline-create-"+code, code, related, value))
		moveProof(t, engine, reviewer, "baseline-review-"+code, proof.ID, proof.Version, "review", http.StatusOK)
		acceptedData, _ := moveProof(t, engine, reviewer, "baseline-accept-"+code, proof.ID, proof.Version+1, "accepted", http.StatusOK)
		// 首份校样没有历史基准（pending），其后的校样必须在容差内通过。
		if index > 0 {
			accepted := proofView(t, append([]byte(`{"data":`), append(acceptedData, []byte("}")...)...))
			if accepted.GateStatus != "passed" {
				t.Fatalf("baseline proof %s gate = %s, want passed", code, accepted.GateStatus)
			}
		}
	}

	// 偏差 4.0 远超常规容差 1.5：创建后应自动转入待复核并持久化阻断快照。
	drifted := proofView(t, createProof(t, engine, operator, "drift-create", "CP-DRIFT-D1", related, 14.0))
	if drifted.Status != "review" || drifted.GateStatus != "blocked" || drifted.GateSampleSize != 3 {
		t.Fatalf("drifted proof not routed to blocked review: %+v", drifted)
	}
	if drifted.GateBaseline != 10.0 || drifted.GateDeviation != 4.0 || drifted.GateTolerance != 1.5 || drifted.GateBlockReason == "" {
		t.Fatalf("drifted proof gate snapshot mismatch: %+v", drifted)
	}
	// 复核员无法直接接受超限校样；错误消息必须带阻断原因。
	_, blockedMessage := moveProof(t, engine, reviewer, "drift-accept-blocked", drifted.ID, drifted.Version, "accepted", http.StatusUnprocessableEntity)
	if !strings.Contains(string(blockedMessage), "容差") {
		t.Fatalf("block message missing tolerance reason: %s", blockedMessage)
	}

	// 复核前把读数修正回容差范围；编辑重算后门禁放行，接受成功（接受前再次重算）。
	updatePayload := driftPayload("CP-DRIFT-D1", related, 10.3)
	updatePayload["expectedVersion"] = drifted.Version
	status, body := perform(t, engine, http.MethodPut, "/api/proofs/"+uintString(drifted.ID), operator, "drift-fix", updatePayload)
	if status != http.StatusOK {
		t.Fatalf("fix drifted proof status = %d body=%s", status, body)
	}
	fixed := proofView(t, body)
	if fixed.GateStatus != "passed" {
		t.Fatalf("fixed proof gate = %s, want passed", fixed.GateStatus)
	}
	moveProof(t, engine, reviewer, "drift-accept-fixed", drifted.ID, fixed.Version, "accepted", http.StatusOK)

	// 取代门禁：旧校样仍可接受，随后创建更新校样，旧校样即被取代而禁止接受。
	older := proofView(t, createProof(t, engine, operator, "older-create", "CP-DRIFT-O1", related, 10.1))
	moveProof(t, engine, reviewer, "older-review", older.ID, older.Version, "review", http.StatusOK)
	_ = proofView(t, createProof(t, engine, operator, "newer-create", "CP-DRIFT-N1", related, 10.05))
	_, supersededMessage := moveProof(t, engine, reviewer, "older-accept-superseded", older.ID, older.Version+1, "accepted", http.StatusUnprocessableEntity)
	if !strings.Contains(string(supersededMessage), "取代") {
		t.Fatalf("superseded block message mismatch: %s", supersededMessage)
	}

	// 无历史基准时门禁为 pending，不阻断首份校样的正常复核接受。
	freshRelated := "REL-DRIFT-NONE"
	fresh := proofView(t, createProof(t, engine, operator, "fresh-create", "CP-DRIFT-F1", freshRelated, 50.0))
	if fresh.Status != "captured" || fresh.GateStatus != "pending" {
		t.Fatalf("fresh proof expected captured/pending, got: %+v", fresh)
	}
	moveProof(t, engine, reviewer, "fresh-review", fresh.ID, fresh.Version, "review", http.StatusOK)
	moveProof(t, engine, reviewer, "fresh-accept", fresh.ID, fresh.Version+1, "accepted", http.StatusOK)

	// 校准摘要刷新后可读，包含基准组（最近五份已接受中位数）与待复核阻断计数。
	status, body = perform(t, engine, http.MethodGet, "/api/proofs/calibration-summary", operator, "summary-read", nil)
	if status != http.StatusOK {
		t.Fatalf("calibration summary status = %d body=%s", status, body)
	}
	summary := decodeData[struct {
		OpenBlocked int64 `json:"openBlocked"`
		Groups      []struct {
			RelatedCode string  `json:"relatedCode"`
			Baseline    float64 `json:"baseline"`
			OpenBlocked int     `json:"openBlocked"`
		} `json:"groups"`
	}](t, body)
	found := false
	for _, group := range summary.Groups {
		if group.RelatedCode == related {
			found = true
			// 此时该组已接受 4 份（10.0/10.2/9.8/10.3），中位数为 10.1。
			if group.Baseline != 10.1 {
				t.Fatalf("group baseline = %v, want 10.1", group.Baseline)
			}
		}
	}
	if !found {
		t.Fatalf("calibration summary missing baseline group %s: %+v", related, summary.Groups)
	}
	if summary.OpenBlocked < 1 {
		t.Fatalf("openBlocked = %d, want at least 1", summary.OpenBlocked)
	}
}
