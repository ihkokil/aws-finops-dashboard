package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws-finops-dashboard/collector/internal/models"
)

// WriteMarkdown generates an executive Markdown summary report.
func WriteMarkdown(report *models.Report, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	dateStr := report.GeneratedAt.Format("2006-01-02")
	filename := fmt.Sprintf("report-%s.md", dateStr)
	filePath := filepath.Join(outputDir, filename)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# AWS FinOps Report — %s\n", dateStr))
	sb.WriteString(fmt.Sprintf("**Account:** `%s` | **Region:** `%s` | **Period:** %d Days\n\n", report.AccountID, report.Region, report.PeriodDays))

	// Executive Summary Table
	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Monthly Spend | $%.2f |\n", report.TotalMonthlySpend))
	sb.WriteString(fmt.Sprintf("| Identified Waste | $%.2f (%.1f%%) |\n", report.TotalMonthlyWaste, report.WastePercentage))
	sb.WriteString(fmt.Sprintf("| Annual Waste Projection | $%.2f |\n", report.AnnualWasteProjection))
	sb.WriteString(fmt.Sprintf("| Realistic Potential Savings (70%%) | $%.2f/month |\n", report.PotentialSavings))
	sb.WriteString(fmt.Sprintf("| Critical Findings (>$100/mo) | %d |\n", report.FindingsBySeverity[models.SeverityCritical]))
	sb.WriteString(fmt.Sprintf("| High Severity Findings (>$50/mo) | %d |\n\n", report.FindingsBySeverity[models.SeverityHigh]))

	// Findings grouped by severity
	criticals := filterBySeverity(report.Findings, models.SeverityCritical)
	highs := filterBySeverity(report.Findings, models.SeverityHigh)
	mediums := filterBySeverity(report.Findings, models.SeverityMedium)

	if len(criticals) > 0 {
		sb.WriteString("## 🔴 Critical Findings (>$100/month waste)\n\n")
		for i, f := range criticals {
			sb.WriteString(fmt.Sprintf("### %d. %s — %s\n", i+1, f.Recommendation, f.ResourceName))
			sb.WriteString(fmt.Sprintf("- **Monthly Waste:** $%.2f\n", f.MonthlyWasteCost))
			sb.WriteString(fmt.Sprintf("- **Resource ID:** `%s` (%s)\n", f.ResourceID, f.ResourceType))
			sb.WriteString(fmt.Sprintf("- **Recommendation:** %s\n", f.Recommendation))
			if len(f.RemediationSteps) > 0 {
				sb.WriteString("- **Remediation:**\n```bash\n")
				for _, step := range f.RemediationSteps {
					sb.WriteString(fmt.Sprintf("%s\n", step))
				}
				sb.WriteString("```\n")
			}
			sb.WriteString("\n")
		}
	}

	if len(highs) > 0 {
		sb.WriteString("## 🟠 High Severity Findings (>$50/month waste)\n\n")
		for i, f := range highs {
			sb.WriteString(fmt.Sprintf("### %d. %s (%s)\n", i+1, f.ResourceName, f.ResourceType))
			sb.WriteString(fmt.Sprintf("- **Monthly Waste:** $%.2f\n", f.MonthlyWasteCost))
			sb.WriteString(fmt.Sprintf("- **Recommendation:** %s\n\n", f.Recommendation))
		}
	}

	if len(mediums) > 0 {
		sb.WriteString("## 🟡 Medium Severity Findings (>$10/month waste)\n\n")
		for i, f := range mediums {
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`): $%.2f/month — %s\n", f.ResourceName, f.ResourceType, f.MonthlyWasteCost, f.Recommendation))
		}
		sb.WriteString("\n")
	}

	// Savings Plan Utilization
	sb.WriteString("## Savings Plan & Reserved Instance Utilization\n\n")
	sb.WriteString(fmt.Sprintf("- **Savings Plan Utilization:** %.1f%%\n", report.SavingsPlanUtilization.UtilizationPercentage))
	sb.WriteString(fmt.Sprintf("- **Unutilized Commitment Waste:** $%.2f/month\n", report.SavingsPlanUtilization.UnutilizedCommitment))
	sb.WriteString(fmt.Sprintf("- **Reserved Instance Coverage:** %.1f%%\n", report.RIUtilization.CoveragePercentage))
	sb.WriteString(fmt.Sprintf("- **RI Unused Hours:** %.0f hours\n\n", report.RIUtilization.UnusedHours))

	// Top 10 Services
	if len(report.TopServices) > 0 {
		sb.WriteString("## Top 10 Services by Spend\n\n")
		sb.WriteString("| Service | Monthly Cost | Usage Quantity |\n")
		sb.WriteString("|---------|--------------|----------------|\n")
		limit := 10
		if len(report.TopServices) < limit {
			limit = len(report.TopServices)
		}
		for i := 0; i < limit; i++ {
			s := report.TopServices[i]
			sb.WriteString(fmt.Sprintf("| %s | $%.2f | %.0f |\n", s.ServiceName, s.UnblendedCost, s.UsageQuantity))
		}
		sb.WriteString("\n")
	}

	// 30-Day Forecast
	if report.Forecast.MeanValue > 0 {
		sb.WriteString("## 30-Day Forecast\n\n")
		sb.WriteString(fmt.Sprintf("- **Predicted Spend:** $%.2f\n", report.Forecast.MeanValue))
		sb.WriteString(fmt.Sprintf("- **Confidence Interval:** $%.2f – $%.2f\n\n", report.Forecast.PredictionIntervalLower, report.Forecast.PredictionIntervalUpper))
	}

	content := sb.String()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write Markdown report file: %w", err)
	}

	// Also write latest
	latestPath := filepath.Join(outputDir, "report-latest.md")
	_ = os.WriteFile(latestPath, []byte(content), 0644)

	return filePath, nil
}

func filterBySeverity(findings []models.Finding, severity string) []models.Finding {
	var result []models.Finding
	for _, f := range findings {
		if f.Severity == severity {
			result = append(result, f)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].MonthlyWasteCost > result[j].MonthlyWasteCost
	})
	return result
}
