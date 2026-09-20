package model

import "time"

// ColorProof models 色彩校样 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type ColorProof struct {
	BaseModel
	Facility    string    `json:"facility" gorm:"size:120;index"`
	Owner       string    `json:"owner" gorm:"size:120;index"`
	Category    string    `json:"category" gorm:"size:80;index"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time `json:"effectiveAt"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64;index"`

	// Drift gate evaluation, populated whenever a proof is offered for review.
	// Baseline is the median of the most recent accepted proofs for the same
	// relatedCode/category; zero values mean no historical baseline exists yet.
	DriftBaseline    *float64   `json:"driftBaseline,omitempty" gorm:"index"`
	DriftDeviation   *float64   `json:"driftDeviation,omitempty"`
	DriftTolerance   *float64   `json:"driftTolerance,omitempty"`
	DriftSampleSize  int        `json:"driftSampleSize"`
	DriftBlocked     bool       `json:"driftBlocked" gorm:"index"`
	DriftBlockReason string     `json:"driftBlockReason,omitempty" gorm:"size:500"`
	DriftEvaluatedAt *time.Time `json:"driftEvaluatedAt,omitempty"`
}

func (item *ColorProof) GetBase() *BaseModel { return &item.BaseModel }

func (item ColorProof) TableName() string { return "color_proofs" }

const (
	ColorProofInitialStatus = "captured"
	// ColorProofReviewPending holds proofs whose drift gate failed: they need a
	// fresh reviewer re-evaluation before they can be accepted or rejected.
	ColorProofReviewPendingStatus = "review_pending"
)

// Drift gate tolerances keyed by proof category.
const (
	ColorProofCategoryRoutine = "常规"
	ColorProofCategoryKey     = "重点"
	ColorProofCategoryRecheck = "复核"
)

var ColorProofDriftTolerance = map[string]float64{
	ColorProofCategoryRoutine: 1.5,
	ColorProofCategoryKey:     1.0,
	ColorProofCategoryRecheck: 0.8,
}

// ColorProofDriftSampleLimit is the number of recent accepted proofs used to
// build the median baseline.
const ColorProofDriftSampleLimit = 5
