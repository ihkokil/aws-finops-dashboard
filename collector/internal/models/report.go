package models

import "time"

// Report is the root aggregation structure containing all collected metrics and findings.
type Report struct {
	GeneratedAt           time.Time              `json:"generated_at"`
	AccountID             string                 `json:"account_id"`
	Region                string                 `json:"region"`
	PeriodDays            int                    `json:"period_days"`
	TotalMonthlySpend     float64                `json:"total_monthly_spend"`
	TotalMonthlyWaste     float64                `json:"total_monthly_waste"`
	WastePercentage       float64                `json:"waste_percentage"`
	AnnualWasteProjection float64                `json:"annual_waste_projection"`
	PotentialSavings      float64                `json:"potential_savings"`
	FindingsBySeverity    map[string]int         `json:"findings_by_severity"`
	FindingsByCategory    map[string]int         `json:"findings_by_category"`
	Findings              []Finding              `json:"findings"`
	SavingsPlanUtilization SavingsPlanUtilization `json:"savings_plan_utilization"`
	RIUtilization         RIUtilization          `json:"ri_utilization"`
	Forecast              ForecastResult         `json:"forecast"`
	TopServices           []ServiceCost          `json:"top_services"`
	DailyCosts            []DailyCost            `json:"daily_costs"`
}

// CalculateSummaryStats updates summary numbers on the report.
func (r *Report) CalculateSummaryStats(minSavings float64) {
	r.FindingsBySeverity = map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityMedium:   0,
		SeverityLow:      0,
	}
	r.FindingsByCategory = map[string]int{
		CategoryIdle:        0,
		CategoryRightsizing: 0,
		CategoryStorage:     0,
		CategoryNetwork:     0,
	}

	filteredFindings := make([]Finding, 0)
	var totalWaste float64

	for _, f := range r.Findings {
		if f.EstimatedSavings < minSavings {
			continue
		}
		filteredFindings = append(filteredFindings, f)
		r.FindingsBySeverity[f.Severity]++
		r.FindingsByCategory[f.Category]++
		totalWaste += f.MonthlyWasteCost
	}

	r.Findings = filteredFindings
	r.TotalMonthlyWaste = totalWaste
	r.AnnualWasteProjection = totalWaste * 12.0

	// Realistic savings expectation is ~70% of total waste
	r.PotentialSavings = totalWaste * 0.70

	if r.TotalMonthlySpend > 0 {
		r.WastePercentage = (r.TotalMonthlyWaste / r.TotalMonthlySpend) * 100.0
	} else {
		r.WastePercentage = 0.0
	}
}
