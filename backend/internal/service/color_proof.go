package service

import (
	"context"
	"fmt"
	"math"
	"sort"
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
	return s.repository.List(ctx, query)
}

func (s *colorProofService) Get(ctx context.Context, id uint) (model.ColorProof, error) {
	return s.repository.Get(ctx, id)
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
	_ = s.security.Audit(ctx, actor, requestID, "create", "ColorProof", item.ID, "", item.Status, "created 色彩校样")
	return item, nil
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
	// Business edits invalidate a previous drift evaluation: proofs waiting in
	// the review queue must be re-gated with the new readings before acceptance.
	if current.Status == model.ColorProofReviewPendingStatus || current.Status == "review" {
		if err := s.refreshDriftGate(ctx, &current); err != nil {
			return model.ColorProof{}, err
		}
	} else {
		resetDriftEvaluation(&current)
	}
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ColorProof{}, fmt.Errorf("update 色彩校样: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ColorProof", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

// refreshDriftGate recomputes the median-baseline drift evaluation and routes a
// failing proof to the pending-review queue. It never short-circuits when no
// accepted history exists yet, so first-of-series proofs can proceed normally.
func (s *colorProofService) refreshDriftGate(ctx context.Context, current *model.ColorProof) error {
	history, err := s.repository.RecentAccepted(ctx, current.RelatedCode, current.Category, current.ID, model.ColorProofDriftSampleLimit)
	if err != nil {
		return fmt.Errorf("load drift history: %w", err)
	}
	applyDriftEvaluation(current, evaluateDrift(history, current.Category, current.MetricValue))
	return nil
}

func (s *colorProofService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	target := strings.TrimSpace(input.Status)
	// Acceptance, rejection and any movement out of the drift pending queue are
	// reviewer-only decisions.
	reviewerAction := target == "accepted" || target == "rejected" ||
		current.Status == "accepted" || current.Status == "rejected" ||
		current.Status == model.ColorProofReviewPendingStatus
	if reviewerAction && !canReview(role) {
		return model.ColorProof{}, ErrForbidden
	}
	if !constants.CanTransition(constants.ColorProofTransitions, current.Status, target) {
		return model.ColorProof{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status

	switch {
	case target == "review":
		// Recompute the drift gate before the proof enters (or re-enters) review.
		if err := s.refreshDriftGate(ctx, &current); err != nil {
			return model.ColorProof{}, err
		}
		if current.DriftBlocked {
			// Over-limit proofs are diverted to the pending-review queue; the
			// stored evaluation keeps baseline, deviation and the block reason.
			current.Status = model.ColorProofReviewPendingStatus
			break
		}
		current.Status = target
	case target == "accepted":
		// Re-evaluate on accept: refuse stale over-limit proofs and proofs that a
		// newer校样 for the same related code has superseded.
		if err := s.refreshDriftGate(ctx, &current); err != nil {
			return model.ColorProof{}, err
		}
		if current.DriftBlocked {
			return model.ColorProof{}, fmt.Errorf("%w: %s", ErrDriftBlocked, current.DriftBlockReason)
		}
		superseded, err := s.repository.HasNewerActiveProof(ctx, current.RelatedCode, current.EffectiveAt, current.ID)
		if err != nil {
			return model.ColorProof{}, fmt.Errorf("check superseded proof: %w", err)
		}
		if superseded {
			return model.ColorProof{}, ErrProofSuperseded
		}
		current.Status = target
	default:
		current.Status = target
		if target == model.ColorProofInitialStatus {
			resetDriftEvaluation(&current)
		}
	}

	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ColorProof{}, fmt.Errorf("transition 色彩校样: %w", err)
	}
	auditDetail := input.Reason
	if current.Status == model.ColorProofReviewPendingStatus {
		auditDetail = strings.TrimSpace(input.Reason + " | 漂移门禁阻断: " + current.DriftBlockReason)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "ColorProof", id, before, current.Status, auditDetail); err != nil {
		return model.ColorProof{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
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

// driftEvaluation captures one run of the colour drift gate against the median
// of the most recent accepted proofs sharing the same related code and category.
type driftEvaluation struct {
	baseline    *float64
	deviation   *float64
	tolerance   float64
	sampleSize  int
	blocked     bool
	blockReason string
}

// evaluateDrift builds the median baseline and decides whether the proof's
// measured value stays inside the category tolerance.
func evaluateDrift(history []model.ColorProof, category string, metricValue float64) driftEvaluation {
	result := driftEvaluation{tolerance: driftToleranceFor(category)}
	result.sampleSize = len(history)
	if len(history) == 0 {
		return result
	}
	values := make([]float64, 0, len(history))
	for _, item := range history {
		values = append(values, item.MetricValue)
	}
	sort.Float64s(values)
	baseline := medianValue(values)
	deviation := math.Abs(metricValue - baseline)
	result.baseline = &baseline
	result.deviation = &deviation
	result.blocked = deviation > result.tolerance
	if result.blocked {
		result.blockReason = fmt.Sprintf(
			"漂移偏差 %.2f 超过%s类别容差 %.2f（基准 %.2f，取样 %d 份已接受校样）",
			deviation, category, result.tolerance, baseline, result.sampleSize,
		)
	}
	return result
}

// driftToleranceFor resolves the gate tolerance by proof category. Unknown
// categories fall back to the routine tolerance.
func driftToleranceFor(category string) float64 {
	if tolerance, ok := model.ColorProofDriftTolerance[strings.TrimSpace(category)]; ok {
		return tolerance
	}
	return model.ColorProofDriftTolerance[model.ColorProofCategoryRoutine]
}

func medianValue(values []float64) float64 {
	count := len(values)
	if count == 0 {
		return 0
	}
	if count%2 == 1 {
		return values[count/2]
	}
	return (values[count/2-1] + values[count/2]) / 2
}

// applyDriftEvaluation persists a fresh drift evaluation onto a proof.
func applyDriftEvaluation(proof *model.ColorProof, evaluation driftEvaluation) {
	now := time.Now().UTC()
	proof.DriftBaseline = evaluation.baseline
	proof.DriftDeviation = evaluation.deviation
	proof.DriftTolerance = &evaluation.tolerance
	proof.DriftSampleSize = evaluation.sampleSize
	proof.DriftBlocked = evaluation.blocked
	proof.DriftBlockReason = evaluation.blockReason
	proof.DriftEvaluatedAt = &now
}

// resetDriftEvaluation clears a stale gate result, e.g. when a proof is sent
// back to captured for rework before a fresh submission.
func resetDriftEvaluation(proof *model.ColorProof) {
	proof.DriftBaseline = nil
	proof.DriftDeviation = nil
	proof.DriftTolerance = nil
	proof.DriftSampleSize = 0
	proof.DriftBlocked = false
	proof.DriftBlockReason = ""
	proof.DriftEvaluatedAt = nil
}

// CalibrationSummary builds the readable drift-gate snapshot after a refresh.
// Proofs are grouped by related code + category so each group reports its own
// median baseline, pending queue and superseded records.
func (s *colorProofService) CalibrationSummary(ctx context.Context) (dto.CalibrationSummary, error) {
	items, err := s.repository.ListAll(ctx)
	if err != nil {
		return dto.CalibrationSummary{}, fmt.Errorf("load proofs for calibration summary: %w", err)
	}
	type groupKey struct{ relatedCode, category string }
	groups := make(map[groupKey]*dto.CalibrationGroupSummary)
	latest := make(map[groupKey]*model.ColorProof)
	summary := dto.CalibrationSummary{Groups: make([]dto.CalibrationGroupSummary, 0)}
	for i := range items {
		item := &items[i]
		summary.TotalProofs++
		if item.Status == "accepted" {
			summary.AcceptedProofs++
		}
		if item.Status == model.ColorProofReviewPendingStatus {
			summary.PendingProofs++
		}
		if item.DriftBlocked {
			summary.BlockedProofs++
		}
		key := groupKey{relatedCode: item.RelatedCode, category: item.Category}
		group, exists := groups[key]
		if !exists {
			tolerance := driftToleranceFor(item.Category)
			group = &dto.CalibrationGroupSummary{
				RelatedCode: item.RelatedCode, Category: item.Category, Tolerance: tolerance,
			}
			groups[key] = group
		}
		if item.Status == model.ColorProofReviewPendingStatus {
			group.PendingProofs++
		}
		// A proof is superseded once a newer, non-rejected proof for the same
		// related code exists while this one has not left the review queue.
		if item.Status != "accepted" && item.Status != "rejected" {
			superseded, err := s.repository.HasNewerActiveProof(ctx, item.RelatedCode, item.EffectiveAt, item.ID)
			if err != nil {
				return dto.CalibrationSummary{}, fmt.Errorf("check superseded proofs: %w", err)
			}
			if superseded {
				group.SupersededProofs++
				summary.SupersededProofs++
			}
		}
		current, seen := latest[key]
		if !seen || item.EffectiveAt.After(current.EffectiveAt) ||
			(item.EffectiveAt.Equal(current.EffectiveAt) && item.ID > current.ID) {
			latest[key] = item
		}
	}
	for key := range groups {
		group := groups[key]
		history, err := s.repository.RecentAccepted(ctx, key.relatedCode, key.category, 0, model.ColorProofDriftSampleLimit)
		if err != nil {
			return dto.CalibrationSummary{}, fmt.Errorf("load drift baseline: %w", err)
		}
		group.SampleSize = len(history)
		if len(history) > 0 {
			values := make([]float64, 0, len(history))
			for _, item := range history {
				values = append(values, item.MetricValue)
			}
			sort.Float64s(values)
			baseline := medianValue(values)
			group.Baseline = &baseline
		}
		if head := latest[key]; head != nil {
			group.LatestProofID = head.ID
			group.LatestBlocked = head.DriftBlocked
			group.LatestDeviation = head.DriftDeviation
		}
		summary.Groups = append(summary.Groups, *group)
	}
	sort.Slice(summary.Groups, func(i, j int) bool {
		if summary.Groups[i].RelatedCode != summary.Groups[j].RelatedCode {
			return summary.Groups[i].RelatedCode < summary.Groups[j].RelatedCode
		}
		return summary.Groups[i].Category < summary.Groups[j].Category
	})
	return summary, nil
}
