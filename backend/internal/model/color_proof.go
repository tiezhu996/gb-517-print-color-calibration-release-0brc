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

	// 漂移门禁快照，由 service 层在创建、编辑与接受前统一重算并持久化，
	// 保证刷新后仍可直接读出基准、偏差与阻断原因。
	GateBaseline    *float64 `json:"gateBaseline" gorm:"column:gate_baseline"`
	GateDeviation   *float64 `json:"gateDeviation" gorm:"column:gate_deviation"`
	GateTolerance   *float64 `json:"gateTolerance" gorm:"column:gate_tolerance"`
	GateSampleSize  int      `json:"gateSampleSize" gorm:"not null;default:0"`
	GateStatus      string   `json:"gateStatus" gorm:"size:16;index;not null;default:pending"`
	GateBlockReason string   `json:"gateBlockReason" gorm:"size:300"`
	SupersededByID  *uint    `json:"supersededById" gorm:"column:superseded_by_id;index"`

	// Superseded 是非持久化的即时标记，接受前按库内最新情况重算，避免
	// 取代方被删除或拒绝后仍错误阻断。
	Superseded bool `json:"superseded" gorm:"-"`
}

func (item *ColorProof) GetBase() *BaseModel { return &item.BaseModel }

func (item ColorProof) TableName() string { return "color_proofs" }

var ColorProofInitialStatus = "captured"
