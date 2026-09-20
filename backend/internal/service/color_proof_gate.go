package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
)

// ErrDriftBlocked 表示校样在接受前的漂移门禁复算中未通过（读数超限或已被取代）。
type ErrDriftBlocked struct {
	Reason string
}

func (e *ErrDriftBlocked) Error() string {
	return "color proof drift gate blocked: " + e.Reason
}

// gateResult 是一次漂移评估的结果，同时作为持久化到 ColorProof 的门禁快照来源。
type gateResult struct {
	baseline     *float64
	deviation    *float64
	tolerance    *float64
	sampleSize   int
	status       string
	blockReason  string
	supersededBy *uint
}

// medianFloat 返回读数中位数；样本数为偶数时取中间两数的平均。
func medianFloat(values []float64) float64 {
	sorted := append(make([]float64, 0, len(values)), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func floatPointer(value float64) *float64 { return &value }

func uintPointer(value uint) *uint { return &value }

// evaluateDriftGate 按关联编码与类别取最近五份已接受校样，以中位数为基准评估当前校样。
// 返回的结果尚未写库，由调用方决定如何持久化以及是否阻断状态迁移。
func (s *colorProofService) evaluateDriftGate(ctx context.Context, proof *model.ColorProof) (gateResult, error) {
	relatedCode := strings.TrimSpace(proof.RelatedCode)
	category := strings.TrimSpace(proof.Category)
	result := gateResult{status: constants.ColorProofGatePending}

	// 关联编码缺失时无法定位基准组，保持待评估，不参与漂移阻断。
	if relatedCode == "" || !constants.IsDriftCategory(category) {
		return result, nil
	}

	tolerance := constants.DriftToleranceFor(category)
	result.tolerance = floatPointer(tolerance)

	history, err := s.repository.ListRecentAccepted(ctx, relatedCode, category, proof.ID, constants.DriftBaselineSample)
	if err != nil {
		return gateResult{}, fmt.Errorf("load drift baseline: %w", err)
	}
	if len(history) > 0 {
		values := make([]float64, 0, len(history))
		for _, accepted := range history {
			values = append(values, accepted.MetricValue)
		}
		baseline := medianFloat(values)
		deviation := math.Abs(proof.MetricValue - baseline)
		result.baseline = floatPointer(baseline)
		result.deviation = floatPointer(deviation)
		result.sampleSize = len(history)
		if deviation > tolerance {
			result.status = constants.ColorProofGateBlocked
			result.blockReason = fmt.Sprintf(
				"读数偏差 %.2f %s 超过「%s」类别容差 %.1f（基准 %.2f，样本 %d 份）",
				deviation, proof.MetricUnit, category, tolerance, baseline, len(history))
		} else {
			result.status = constants.ColorProofGatePassed
		}
	}

	// 同一关联编码与类别下出现更新的有效校样时，当前校样视为已被取代。
	newer, exists, err := s.repository.FindActiveNewerProof(ctx, relatedCode, category, proof.CreatedAt, proof.ID)
	if err != nil {
		return gateResult{}, fmt.Errorf("check superseded proof: %w", err)
	}
	if exists {
		result.supersededBy = uintPointer(newer.ID)
		result.status = constants.ColorProofGateBlocked
		supersededReason := fmt.Sprintf("已被更新校样 %s 取代", newer.Code)
		if result.blockReason == "" {
			result.blockReason = supersededReason
		} else {
			result.blockReason += "；" + supersededReason
		}
	}
	return result, nil
}

// applyGateResult 把评估快照写回校样实体（不落库），随后通过正常 Update 或
// UpdateGateFields 持久化。
func applyGateResult(proof *model.ColorProof, result gateResult) {
	proof.GateBaseline = result.baseline
	proof.GateDeviation = result.deviation
	proof.GateTolerance = result.tolerance
	proof.GateSampleSize = result.sampleSize
	proof.GateStatus = result.status
	proof.GateBlockReason = result.blockReason
	proof.SupersededByID = result.supersededBy
}

// persistGateSnapshot 仅更新门禁快照列，不影响乐观锁版本号。
func (s *colorProofService) persistGateSnapshot(ctx context.Context, id uint, result gateResult) error {
	fields := map[string]any{
		"gate_baseline":     result.baseline,
		"gate_deviation":    result.deviation,
		"gate_tolerance":    result.tolerance,
		"gate_sample_size":  result.sampleSize,
		"gate_status":       result.status,
		"gate_block_reason": result.blockReason,
		"superseded_by_id":  result.supersededBy,
	}
	return s.repository.UpdateGateFields(ctx, id, fields)
}

// CalibrationSummary 汇总各「关联编码 + 类别」基准组的漂移门禁状态，供刷新后
// 的工作台读取校准摘要。
func (s *colorProofService) CalibrationSummary(ctx context.Context) (dto.CalibrationSummary, error) {
	counts, err := s.repository.CountByStatus(ctx)
	if err != nil {
		return dto.CalibrationSummary{}, err
	}
	var total int64
	for _, count := range counts {
		total += count
	}
	openBlocked, err := s.repository.CountOpenBlocked(ctx)
	if err != nil {
		return dto.CalibrationSummary{}, err
	}
	accepted, err := s.repository.ListByStatus(ctx, "accepted")
	if err != nil {
		return dto.CalibrationSummary{}, err
	}
	review, err := s.repository.ListByStatus(ctx, "review")
	if err != nil {
		return dto.CalibrationSummary{}, err
	}

	type groupKey struct{ relatedCode, category string }
	type groupAccumulator struct {
		recent      []model.ColorProof
		accepted    int
		openBlocked int
	}
	groups := map[groupKey]*groupAccumulator{}
	for _, proof := range accepted {
		key := groupKey{proof.RelatedCode, proof.Category}
		group := groups[key]
		if group == nil {
			group = &groupAccumulator{}
			groups[key] = group
		}
		// accepted 已按 id DESC 排序，取前 DriftBaselineSample 份与门禁基准口径一致。
		if len(group.recent) < constants.DriftBaselineSample {
			group.recent = append(group.recent, proof)
		}
		group.accepted++
	}
	for _, proof := range review {
		if proof.GateStatus != constants.ColorProofGateBlocked {
			continue
		}
		key := groupKey{proof.RelatedCode, proof.Category}
		group := groups[key]
		if group == nil {
			group = &groupAccumulator{}
			groups[key] = group
		}
		group.openBlocked++
	}

	summaries := make([]dto.CalibrationGroupSummary, 0, len(groups))
	for key, group := range groups {
		summary := dto.CalibrationGroupSummary{
			RelatedCode: key.relatedCode,
			Category:    key.category,
			Tolerance:   constants.DriftToleranceFor(key.category),
			Accepted:    group.accepted,
			OpenBlocked: group.openBlocked,
		}
		if len(group.recent) > 0 {
			values := make([]float64, 0, len(group.recent))
			for _, proof := range group.recent {
				values = append(values, proof.MetricValue)
			}
			summary.Baseline = medianFloat(values)
			summary.SampleSize = len(group.recent)
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].RelatedCode != summaries[j].RelatedCode {
			return summaries[i].RelatedCode < summaries[j].RelatedCode
		}
		return summaries[i].Category < summaries[j].Category
	})

	return dto.CalibrationSummary{
		TotalProofs:    total,
		AcceptedProofs: counts["accepted"],
		OpenBlocked:    openBlocked,
		Groups:         summaries,
	}, nil
}

// enrichSuperseded 按持久化的取代指针批量刷新非持久化 Superseded 标记，
// 列表接口用单次回填查询即可展示最新取代状态。
func (s *colorProofService) enrichSuperseded(ctx context.Context, proofs []model.ColorProof) error {
	ids := make(map[uint]struct{})
	for _, proof := range proofs {
		if proof.SupersededByID != nil {
			ids[*proof.SupersededByID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	idList := make([]uint, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	active, err := s.repository.ListByIDs(ctx, idList)
	if err != nil {
		return err
	}
	activeIDs := make(map[uint]bool, len(active))
	for _, proof := range active {
		if proof.Status != "rejected" {
			activeIDs[proof.ID] = true
		}
	}
	for index := range proofs {
		if proofs[index].SupersededByID != nil {
			proofs[index].Superseded = activeIDs[*proofs[index].SupersededByID]
		}
	}
	return nil
}
