package constants

// Shared status values are mirrored in frontend/src/types/status.ts. Keeping
// the lists explicit makes state-machine drift visible during code review.

type RunState string

const (
	RunStateSetup    RunState = "setup"
	RunStatePrinting RunState = "printing"
	RunStateProofing RunState = "proofing"
	RunStateHold     RunState = "hold"
	RunStateReleased RunState = "released"
)

var AllRunState = []string{"setup", "printing", "proofing", "hold", "released"}

type DecisionType string

const (
	DecisionTypeRelease    DecisionType = "release"
	DecisionTypeRework     DecisionType = "rework"
	DecisionTypeQuarantine DecisionType = "quarantine"
)

var AllDecisionType = []string{"release", "rework", "quarantine"}

var PressUnitTransitions = map[string]map[string]bool{
	"ready":       {"setup": true, "printing": true},
	"setup":       {"printing": true, "maintenance": true, "ready": true},
	"printing":    {"maintenance": true, "setup": true},
	"maintenance": {"printing": true},
}

var PrintRunTransitions = map[string]map[string]bool{
	"setup":    {"printing": true},
	"printing": {"proofing": true, "hold": true, "setup": true},
	"proofing": {"hold": true, "released": true, "printing": true},
	"hold":     {"proofing": true},
	"released": {"hold": true},
}

var ColorProofTransitions = map[string]map[string]bool{
	"captured": {"review": true},
	"review":   {"accepted": true, "rejected": true, "captured": true},
	"accepted": {"review": true},
	"rejected": {"review": true},
}

var ReleaseDecisionTransitions = map[string]map[string]bool{
	"draft":      {"release": true, "rework": true},
	"release":    {"rework": true, "quarantine": true, "draft": true},
	"rework":     {"quarantine": true, "release": true},
	"quarantine": {"rework": true},
}

func CanTransition(graph map[string]map[string]bool, from, to string) bool {
	targets, exists := graph[from]
	return exists && targets[to]
}

// 色彩校样漂移门禁配置。基准取同一关联编码（RelatedCode）与同一类别下最近
// DriftBaselineSample 份已接受校样读数的中位数，偏差超过类别容差即阻断。
const DriftBaselineSample = 5

// 门禁评估结果，持久化到 ColorProof.GateStatus 供工作台直接展示。
const (
	ColorProofGatePending = "pending" // 尚无已接受基准，未参与漂移判定
	ColorProofGatePassed  = "passed"  // 偏差在类别容差内
	ColorProofGateBlocked = "blocked" // 偏差超限或已被更新校样取代
)

// driftTolerances 按校样类别给出允许的读数绝对偏差；未识别类别回退到最宽的常规容差。
var driftTolerances = map[string]float64{
	"常规": 1.5,
	"重点": 1.0,
	"复核": 0.8,
}

// DriftToleranceFor 返回类别的漂移容差，未知类别按常规容差处理。
func DriftToleranceFor(category string) float64 {
	if tolerance, ok := driftTolerances[category]; ok {
		return tolerance
	}
	return driftTolerances["常规"]
}

// IsDriftCategory 报告类别是否配置了显式漂移容差。
func IsDriftCategory(category string) bool {
	_, ok := driftTolerances[category]
	return ok
}

// AllDriftCategories 返回参与漂移门禁的类别列表。
func AllDriftCategories() []string {
	return []string{"常规", "重点", "复核"}
}
