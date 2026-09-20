package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"github.com/blueship581/print-color-calibration-release/backend/internal/repository"
)

type ColorProofService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ColorProof], error)
	Get(context.Context, uint) (model.ColorProof, error)
	Create(context.Context, dto.CreateColorProof, string, string) (model.ColorProof, error)
	Update(context.Context, uint, dto.UpdateColorProof, string, string) (model.ColorProof, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.ColorProof, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	CalibrationSummary(context.Context) (dto.CalibrationSummary, error)
}

type colorProofService struct {
	repository repository.ColorProofRepository
	security   SecurityService
}

func NewColorProofService(repo repository.ColorProofRepository, security SecurityService) ColorProofService {
	return &colorProofService{repository: repo, security: security}
}

func (s *colorProofService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ColorProof], error) {
	page, err := s.repository.List(ctx, query)
	if err != nil {
		return page, err
	}
	if err := s.enrichSuperseded(ctx, page.Items); err != nil {
		return page, fmt.Errorf("enrich drift gate state: %w", err)
	}
	return page, nil
}

func (s *colorProofService) Get(ctx context.Context, id uint) (model.ColorProof, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return item, err
	}
	items := []model.ColorProof{item}
	if err := s.enrichSuperseded(ctx, items); err != nil {
		return item, fmt.Errorf("enrich drift gate state: %w", err)
	}
	return items[0], nil
}

func (s *colorProofService) Create(ctx context.Context, input dto.CreateColorProof, actor, requestID string) (model.ColorProof, error) {
	if err := validateColorProofBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ColorProof{}, err
	}
	item := model.ColorProof{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ColorProofInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ColorProof{}, fmt.Errorf("create 色彩校样: %w", err)
	}

	// 创建后立即执行漂移门禁：超限时校样直接进入待复核（review），
	// 不再停留在 captured，确保问题读数必须经过人工复核。
	gate, err := s.evaluateDriftGate(ctx, &item)
	if err != nil {
		return model.ColorProof{}, err
	}
	applyGateResult(&item, gate)
	forcedReview := gate.status == constants.ColorProofGateBlocked && item.Status == model.ColorProofInitialStatus
	if forcedReview {
		item.Status = "review"
	}
	gateFields := map[string]any{
		"gate_baseline": gate.baseline, "gate_deviation": gate.deviation,
		"gate_tolerance": gate.tolerance, "gate_sample_size": gate.sampleSize,
		"gate_status": gate.status, "gate_block_reason": gate.blockReason,
		"superseded_by_id": gate.supersededBy,
	}
	if forcedReview {
		gateFields["status"] = "review"
	}
	if err := s.repository.UpdateGateFields(ctx, item.ID, gateFields); err != nil {
		return model.ColorProof{}, fmt.Errorf("persist drift gate: %w", err)
	}

	detail := "created 色彩校样"
	if forcedReview {
		detail = "created 色彩校样；漂移门禁阻断，转入待复核：" + gate.blockReason
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ColorProof", item.ID, "", item.Status, detail)
	return s.Get(ctx, item.ID)
}

func (s *colorProofService) Update(ctx context.Context, id uint, input dto.UpdateColorProof, actor, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	if err := validateColorProofBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ColorProof{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ColorProof{}, fmt.Errorf("update 色彩校样: %w", err)
	}

	// 读数或基准组（关联编码/类别）可能已被修改，编辑成功后必须重算门禁快照。
	gate, err := s.evaluateDriftGate(ctx, &current)
	if err != nil {
		return model.ColorProof{}, err
	}
	applyGateResult(&current, gate)
	if err := s.persistGateSnapshot(ctx, id, gate); err != nil {
		return model.ColorProof{}, fmt.Errorf("persist drift gate: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ColorProof", id, current.Status, current.Status, "updated business fields")
	return s.Get(ctx, id)
}

func (s *colorProofService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	target := strings.TrimSpace(input.Status)
	if (target == "accepted" || target == "rejected" || current.Status == "accepted" || current.Status == "rejected") && !canReview(role) {
		return model.ColorProof{}, ErrForbidden
	}

	// 接受前一律按库内最新情况重算门禁，禁止接受读数超限或已被更新校样取代的记录。
	// 复核员先把读数改回容差范围（编辑触发重算）后再接受即可放行。
	if target == "accepted" {
		gate, err := s.evaluateDriftGate(ctx, &current)
		if err != nil {
			return model.ColorProof{}, err
		}
		applyGateResult(&current, gate)
		if err := s.persistGateSnapshot(ctx, id, gate); err != nil {
			return model.ColorProof{}, fmt.Errorf("persist drift gate: %w", err)
		}
		if gate.status == constants.ColorProofGateBlocked {
			return model.ColorProof{}, &ErrDriftBlocked{Reason: gate.blockReason}
		}
	} else if current.Status == "accepted" || target == "review" {
		// 退回重审或重新进入待复核时刷新快照，让基准、偏差与取代标记保持最新。
		gate, err := s.evaluateDriftGate(ctx, &current)
		if err != nil {
			return model.ColorProof{}, err
		}
		applyGateResult(&current, gate)
		if err := s.persistGateSnapshot(ctx, id, gate); err != nil {
			return model.ColorProof{}, fmt.Errorf("persist drift gate: %w", err)
		}
	}

	if !constants.CanTransition(constants.ColorProofTransitions, current.Status, target) {
		return model.ColorProof{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ColorProof{}, fmt.Errorf("transition 色彩校样: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "ColorProof", id, before, target, input.Reason); err != nil {
		return model.ColorProof{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *colorProofService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ColorProof", id, current.Status, "deleted", "soft deleted 色彩校样")
}

func (s *colorProofService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateColorProofBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
