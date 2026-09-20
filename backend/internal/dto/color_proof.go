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

// CalibrationGroupSummary is the drift baseline view for one relatedCode +
// category combination on the calibration summary refresh.
type CalibrationGroupSummary struct {
	RelatedCode      string   `json:"relatedCode"`
	Category         string   `json:"category"`
	Baseline         *float64 `json:"baseline,omitempty"`
	Tolerance        float64  `json:"tolerance"`
	SampleSize       int      `json:"sampleSize"`
	PendingProofs    int      `json:"pendingProofs"`
	SupersededProofs int      `json:"supersededProofs"`
	LatestProofID    uint     `json:"latestProofId"`
	LatestDeviation  *float64 `json:"latestDeviation,omitempty"`
	LatestBlocked    bool     `json:"latestBlocked"`
}

// CalibrationSummary is the readable drift gate snapshot shown after refresh.
type CalibrationSummary struct {
	TotalProofs      int                       `json:"totalProofs"`
	AcceptedProofs   int                       `json:"acceptedProofs"`
	PendingProofs    int                       `json:"pendingProofs"`
	BlockedProofs    int                       `json:"blockedProofs"`
	SupersededProofs int                       `json:"supersededProofs"`
	Groups           []CalibrationGroupSummary `json:"groups"`
}
