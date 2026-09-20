package dto

import "time"

// CreateColorProof is the public write contract for 色彩校样. Status is deliberately
// omitted so callers cannot bypass the service state machine.
type CreateColorProof struct {
	Code        string    `json:"code" binding:"required,min=2,max=64"`
	Name        string    `json:"name" binding:"required,min=2,max=160"`
	Description string    `json:"description" binding:"max=1000"`
	Facility    string    `json:"facility" binding:"required,max=120"`
	Owner       string    `json:"owner" binding:"required,max=120"`
	Category    string    `json:"category" binding:"required,max=80"`
	RiskLevel   string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt time.Time `json:"effectiveAt" binding:"required"`
	Evidence    string    `json:"evidence" binding:"max=2000"`
	RelatedCode string    `json:"relatedCode" binding:"max=64"`
}

type UpdateColorProof struct {
	ExpectedVersion uint      `json:"expectedVersion" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=160"`
	Description     string    `json:"description" binding:"max=1000"`
	Facility        string    `json:"facility" binding:"required,max=120"`
	Owner           string    `json:"owner" binding:"required,max=120"`
	Category        string    `json:"category" binding:"required,max=80"`
	RiskLevel       string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue     float64   `json:"metricValue"`
	MetricUnit      string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt     time.Time `json:"effectiveAt" binding:"required"`
	Evidence        string    `json:"evidence" binding:"max=2000"`
	RelatedCode     string    `json:"relatedCode" binding:"max=64"`
}

// CalibrationGroupSummary 是某个「关联编码 + 类别」基准组的校准摘要。
type CalibrationGroupSummary struct {
	RelatedCode string  `json:"relatedCode"`
	Category    string  `json:"category"`
	Tolerance   float64 `json:"tolerance"`
	Baseline    float64 `json:"baseline"`
	SampleSize  int     `json:"sampleSize"`
	Accepted    int     `json:"accepted"`
	OpenBlocked int     `json:"openBlocked"`
}

// CalibrationSummary 是漂移门禁的只读汇总，刷新工作台后可直接读取。
type CalibrationSummary struct {
	TotalProofs    int64                     `json:"totalProofs"`
	AcceptedProofs int64                     `json:"acceptedProofs"`
	OpenBlocked    int64                     `json:"openBlocked"`
	Groups         []CalibrationGroupSummary `json:"groups"`
}
