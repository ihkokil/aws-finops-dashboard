package models

import "time"

const (
	SeverityCritical = "critical" // > $100/mo waste
	SeverityHigh     = "high"     // > $50/mo waste
	SeverityMedium   = "medium"   // > $10/mo waste
	SeverityLow      = "low"      // <= $10/mo waste

	CategoryIdle        = "idle"
	CategoryRightsizing = "rightsizing"
	CategoryStorage     = "storage"
	CategoryNetwork     = "network"
)

// Finding represents a single cost optimization issue or opportunity.
type Finding struct {
	ID                string             `json:"id"`
	Category          string             `json:"category"`           // "idle", "rightsizing", "storage", "network"
	Severity          string             `json:"severity"`           // "critical", "high", "medium", "low"
	ResourceType      string             `json:"resource_type"`      // "ec2", "rds", "ebs", "elb", "nat", "s3"
	ResourceID        string             `json:"resource_id"`
	ResourceName      string             `json:"resource_name"`
	Region            string             `json:"region"`
	AccountID         string             `json:"account_id"`
	Tags              map[string]string  `json:"tags"`
	MonthlyWasteCost  float64            `json:"monthly_waste_cost"`
	AnnualWasteCost   float64            `json:"annual_waste_cost"` // monthly * 12
	EstimatedSavings  float64            `json:"estimated_savings"`
	SavingsPercentage float64            `json:"savings_percentage"`
	Recommendation    string             `json:"recommendation"`
	RemediationSteps  []string           `json:"remediation_steps"`
	Confidence        string             `json:"confidence"` // "high", "medium", "low"
	DetectedAt        time.Time          `json:"detected_at"`
	Metrics           map[string]float64 `json:"metrics"` // raw CloudWatch metrics
}

// DetermineSeverity calculates severity based on monthly waste threshold.
func DetermineSeverity(monthlyWaste float64) string {
	switch {
	case monthlyWaste >= 100.0:
		return SeverityCritical
	case monthlyWaste >= 50.0:
		return SeverityHigh
	case monthlyWaste >= 10.0:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
