package models

// ServiceCost captures spend and usage for a specific AWS service.
type ServiceCost struct {
	ServiceName   string  `json:"service_name"`
	UnblendedCost float64 `json:"unblended_cost"`
	UsageQuantity float64 `json:"usage_quantity"`
	Unit          string  `json:"unit"`
}

// DailyCost represents cost metrics for a single date.
type DailyCost struct {
	Date          string  `json:"date"`
	UnblendedCost float64 `json:"unblended_cost"`
}

// TaggedCost aggregates cost details grouped by tag key/value.
type TaggedCost struct {
	TagValue      string  `json:"tag_value"`
	UnblendedCost float64 `json:"unblended_cost"`
}

// ForecastResult holds predicted spend and confidence boundaries.
type ForecastResult struct {
	StartDate                 string  `json:"start_date"`
	EndDate                   string  `json:"end_date"`
	MeanValue                 float64 `json:"mean_value"`
	PredictionIntervalLower   float64 `json:"prediction_interval_lower"`
	PredictionIntervalUpper   float64 `json:"prediction_interval_upper"`
	PreviousMonthSpend        float64 `json:"previous_month_spend"`
	ForecastVsPreviousPercent float64 `json:"forecast_vs_previous_percent"`
}

// SavingsPlanUtilization details coverage, commitment, and unutilized spend.
type SavingsPlanUtilization struct {
	CoveragePercentage    float64 `json:"coverage_percentage"`
	UtilizationPercentage float64 `json:"utilization_percentage"`
	UnutilizedCommitment  float64 `json:"unutilized_commitment_dollars"`
	EstimatedSavings      float64 `json:"estimated_savings_dollars"`
	TotalCommitment       float64 `json:"total_commitment_dollars"`
	UsedCommitment        float64 `json:"used_commitment_dollars"`
}

// RIUtilization details reservation coverage and unused hours waste.
type RIUtilization struct {
	CoveragePercentage float64            `json:"coverage_percentage"`
	UtilizationPercent float64            `json:"utilization_percent"`
	UnusedHours        float64            `json:"unused_hours"`
	WasteCost          float64            `json:"waste_cost"`
	ServiceUtilization map[string]float64 `json:"service_utilization"`
	ServiceUnusedHours map[string]float64 `json:"service_unused_hours"`
}
