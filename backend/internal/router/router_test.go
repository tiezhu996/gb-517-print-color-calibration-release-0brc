package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/config"
	"github.com/blueship581/print-color-calibration-release/backend/internal/database"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"github.com/blueship581/print-color-calibration-release/backend/internal/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

func driftProof(t *testing.T, engine *gin.Engine, token, code, relatedCode, category string, metricValue float64, effectiveAt time.Time) struct {
	ID      uint   `json:"id"`
	Version uint   `json:"version"`
	Status  string `json:"status"`
} {
	t.Helper()
	payload := map[string]any{
		"code": code, "name": "漂移门禁测试校样 " + code, "description": "drift gate router test",
		"facility": "测试印刷区", "owner": "operator", "category": category,
		"riskLevel": "medium", "metricValue": metricValue, "metricUnit": "dE",
		"effectiveAt": effectiveAt.Format(time.RFC3339Nano), "evidence": "spectrophotometer evidence",
		"relatedCode": relatedCode,
	}
	status, body := perform(t, engine, http.MethodPost, "/api/proofs", token, "drift-create-"+code, payload)
	if status != http.StatusCreated {
		t.Fatalf("create proof %s status = %d body=%s", code, status, body)
	}
	return decodeData[struct {
		ID      uint   `json:"id"`
		Version uint   `json:"version"`
		Status  string `json:"status"`
	}](t, body)
}

func transitionProof(t *testing.T, engine *gin.Engine, token, requestID string, id, version uint, target string) (int, []byte) {
	t.Helper()
	path := "/api/proofs/" + uintString(id) + "/transition"
	payload := map[string]any{"status": target, "expectedVersion": version, "reason": requestID}
	return perform(t, engine, http.MethodPost, path, token, requestID, payload)
}

func seedAcceptedProofs(t *testing.T, db *gorm.DB, prefix, relatedCode, category string, values []float64, baseTime time.Time) {
	t.Helper()
	items := make([]model.ColorProof, 0, len(values))
	for i, value := range values {
		items = append(items, model.ColorProof{
			BaseModel: model.BaseModel{
				Code: fmt.Sprintf("%s-%d", prefix, i+1), Name: "漂移基准校样 " + prefix,
				Status: "accepted", Version: 1,
			},
			Facility: "测试印刷区", Owner: "reviewer", Category: category, RiskLevel: "medium",
			MetricValue: value, MetricUnit: "dE", EffectiveAt: baseTime.Add(time.Duration(i) * time.Hour),
			Evidence: "accepted baseline", RelatedCode: relatedCode,
		})
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatalf("seed accepted proofs: %v", err)
	}
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
	now := time.Now().UTC()

	// Five accepted 常规 proofs (tolerance 1.5) with values 10..14; median = 12.
	seedAcceptedProofs(t, db, "CP-BASE-A", "REL-DRIFT-A", "常规", []float64{10, 11, 12, 13, 14}, now)
	// Five accepted 重点 proofs (tolerance 1.0) with values 20..24; median = 22.
	seedAcceptedProofs(t, db, "CP-BASE-B", "REL-KEY-B", "重点", []float64{20, 21, 22, 23, 24}, now)

	// An in-tolerance proof (12.5, deviation 0.5) passes straight into review.
	within := driftProof(t, engine, operator, "CP-DRIFT-W", "REL-DRIFT-A", "常规", 12.5, now.Add(7*time.Hour))
	if status, _ := transitionProof(t, engine, operator, "operator-cannot-accept", within.ID, within.Version, "accepted"); status != http.StatusForbidden {
		t.Fatalf("operator accept status = %d, want 403", status)
	}
	if status, body := transitionProof(t, engine, operator, "drift-within-review", within.ID, within.Version, "review"); status != http.StatusOK {
		t.Fatalf("in-tolerance review status = %d body=%s", status, body)
	}
	if status, _ := transitionProof(t, engine, reviewer, "drift-within-accept", within.ID, within.Version+1, "accepted"); status != http.StatusOK {
		t.Fatalf("in-tolerance accept status = %d, want 200", status)
	}

	// The newest five accepted are now 11,12,12.5,13,14, so the median moved to
	// 12.5. A reading of 16.0 deviates 3.5 and is diverted to review_pending
	// with baseline, deviation and block reason stored on the record.
	over := driftProof(t, engine, operator, "CP-DRIFT-O", "REL-DRIFT-A", "常规", 16.0, now.Add(8*time.Hour))
	status, body := transitionProof(t, engine, operator, "drift-over-review", over.ID, over.Version, "review")
	if status != http.StatusOK {
		t.Fatalf("over-limit review status = %d body=%s", status, body)
	}
	pending := decodeData[struct {
		Status           string   `json:"status"`
		DriftBlocked     bool     `json:"driftBlocked"`
		DriftBaseline    *float64 `json:"driftBaseline"`
		DriftDeviation   *float64 `json:"driftDeviation"`
		DriftTolerance   *float64 `json:"driftTolerance"`
		DriftSampleSize  int      `json:"driftSampleSize"`
		DriftBlockReason string   `json:"driftBlockReason"`
	}](t, body)
	if pending.Status != "review_pending" || !pending.DriftBlocked || pending.DriftBaseline == nil ||
		math.Abs(*pending.DriftBaseline-12.5) > 1e-9 || pending.DriftDeviation == nil || math.Abs(*pending.DriftDeviation-3.5) > 1e-9 ||
		pending.DriftTolerance == nil || *pending.DriftTolerance != 1.5 || pending.DriftSampleSize != 5 ||
		!strings.Contains(pending.DriftBlockReason, "超过常规类别容差") {
		t.Fatalf("unexpected pending drift evaluation: %+v", pending)
	}

	// Acceptance must be blocked while the proof remains over tolerance.
	if status, body := transitionProof(t, engine, reviewer, "drift-over-accept-blocked", over.ID, over.Version+1, "accepted"); status != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit accept status = %d body=%s, want 422", status, body)
	}

	// A newer proof for the same related code supersedes the pending proof.
	newer := driftProof(t, engine, operator, "CP-DRIFT-N", "REL-DRIFT-A", "常规", 12.8, now.Add(9*time.Hour))
	if status, body := transitionProof(t, engine, operator, "drift-newer-review", newer.ID, newer.Version, "review"); status != http.StatusOK {
		t.Fatalf("newer proof review status = %d body=%s", status, body)
	}
	// Re-gating while still over tolerance keeps the proof in the pending queue.
	status, body = transitionProof(t, engine, reviewer, "drift-pending-recheck-superseded", over.ID, over.Version+1, "review")
	if status != http.StatusOK {
		t.Fatalf("pending re-gate status = %d body=%s", status, body)
	}
	recheck := decodeData[struct {
		Status  string `json:"status"`
		Version uint   `json:"version"`
		Blocked bool   `json:"driftBlocked"`
	}](t, body)
	if recheck.Status != "review_pending" || !recheck.Blocked {
		t.Fatalf("re-gated over-limit proof should stay pending: %+v", recheck)
	}

	// Update the pending proof back within tolerance; the PUT refresh re-runs the
	// gate with optimistic locking intact.
	overVersion := recheck.Version
	updatePayload := map[string]any{
		"expectedVersion": overVersion, "name": "漂移门禁测试校样 CP-DRIFT-O",
		"facility": "测试印刷区", "owner": "operator", "category": "常规",
		"riskLevel": "medium", "metricValue": 12.6, "metricUnit": "dE",
		"effectiveAt": now.Add(8 * time.Hour).Format(time.RFC3339Nano), "evidence": "adjusted reading",
		"relatedCode": "REL-DRIFT-A",
	}
	status, body = perform(t, engine, http.MethodPut, "/api/proofs/"+uintString(over.ID), reviewer, "drift-over-update", updatePayload)
	if status != http.StatusOK {
		t.Fatalf("update pending proof status = %d body=%s", status, body)
	}
	updated := decodeData[struct {
		Version      uint   `json:"version"`
		Status       string `json:"status"`
		DriftBlocked bool   `json:"driftBlocked"`
	}](t, body)
	if updated.Status != "review_pending" {
		t.Fatalf("edited in-tolerance proof should leave pending via re-gate, got status=%s", updated.Status)
	}
	overVersion = updated.Version

	// The newer proof is still active in review, so acceptance is refused.
	status, body = transitionProof(t, engine, reviewer, "drift-pending-review-clear", over.ID, overVersion, "review")
	if status != http.StatusOK {
		t.Fatalf("review after edit status = %d body=%s", status, body)
	}
	cleared := decodeData[struct {
		Status  string `json:"status"`
		Version uint   `json:"version"`
	}](t, body)
	if cleared.Status != "review" {
		t.Fatalf("in-tolerance re-gate should reach review, got %s", cleared.Status)
	}
	overVersion = cleared.Version
	if status, body := transitionProof(t, engine, reviewer, "drift-superseded-accept", over.ID, overVersion, "accepted"); status != http.StatusConflict {
		t.Fatalf("superseded accept status = %d body=%s, want 409", status, body)
	}

	// Once the newer proof is rejected the stale proof is no longer superseded.
	if status, _ := transitionProof(t, engine, reviewer, "drift-newer-reject", newer.ID, newer.Version+1, "rejected"); status != http.StatusOK {
		t.Fatalf("newer proof reject status = %d", status)
	}
	if status, body := transitionProof(t, engine, reviewer, "drift-pending-accept", over.ID, overVersion, "accepted"); status != http.StatusOK {
		t.Fatalf("accept after newer rejected status = %d body=%s", status, body)
	}

	// 重点 category: value 23.2 deviates 1.2 from baseline 22 and exceeds 1.0.
	keyOver := driftProof(t, engine, operator, "CP-KEY-O", "REL-KEY-B", "重点", 23.2, now.Add(6*time.Hour))
	status, body = transitionProof(t, engine, operator, "key-over-review", keyOver.ID, keyOver.Version, "review")
	keyPending := decodeData[struct {
		Status         string   `json:"status"`
		DriftTolerance *float64 `json:"driftTolerance"`
		DriftDeviation *float64 `json:"driftDeviation"`
	}](t, body)
	if status != http.StatusOK || keyPending.Status != "review_pending" ||
		keyPending.DriftTolerance == nil || *keyPending.DriftTolerance != 1.0 ||
		keyPending.DriftDeviation == nil || math.Abs(*keyPending.DriftDeviation-1.2) > 1e-9 {
		t.Fatalf("key category gate result: status=%d %+v", status, keyPending)
	}

	// The refreshed calibration summary is readable and reports the groups.
	status, body = perform(t, engine, http.MethodGet, "/api/proofs/calibration-summary", reviewer, "drift-summary", nil)
	if status != http.StatusOK {
		t.Fatalf("calibration summary status = %d body=%s", status, body)
	}
	summary := decodeData[struct {
		PendingProofs    int `json:"pendingProofs"`
		SupersededProofs int `json:"supersededProofs"`
		Groups           []struct {
			RelatedCode   string   `json:"relatedCode"`
			Category      string   `json:"category"`
			Baseline      *float64 `json:"baseline"`
			Tolerance     float64  `json:"tolerance"`
			PendingProofs int      `json:"pendingProofs"`
		} `json:"groups"`
	}](t, body)
	if summary.PendingProofs != 1 || len(summary.Groups) < 2 {
		t.Fatalf("unexpected calibration summary: %+v", summary)
	}
}
