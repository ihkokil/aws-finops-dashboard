package output

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aws-finops-dashboard/collector/internal/models"
)

// WriteCSV exports findings to a flat CSV file suitable for spreadsheet import.
func WriteCSV(report *models.Report, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := fmt.Sprintf("report-%s.csv", report.GeneratedAt.Format("2006-01-02"))
	filePath := filepath.Join(outputDir, filename)

	file, err := os.OpenFile(filepath.Clean(filePath), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"ID", "Category", "Severity", "ResourceType", "ResourceID", "ResourceName",
		"Region", "AccountID", "MonthlyWasteCost", "AnnualWasteCost",
		"EstimatedSavings", "SavingsPercentage", "Recommendation", "Confidence", "DetectedAt",
	}
	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, f := range report.Findings {
		row := []string{
			f.ID,
			f.Category,
			f.Severity,
			f.ResourceType,
			f.ResourceID,
			f.ResourceName,
			f.Region,
			f.AccountID,
			fmt.Sprintf("%.2f", f.MonthlyWasteCost),
			fmt.Sprintf("%.2f", f.AnnualWasteCost),
			fmt.Sprintf("%.2f", f.EstimatedSavings),
			fmt.Sprintf("%.1f", f.SavingsPercentage),
			f.Recommendation,
			f.Confidence,
			f.DetectedAt.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	// Also write summary row at the bottom
	summaryRow := []string{
		"TOTAL_SUMMARY", "-", "-", "-", "-", "-",
		report.Region, report.AccountID,
		fmt.Sprintf("%.2f", report.TotalMonthlyWaste),
		fmt.Sprintf("%.2f", report.AnnualWasteProjection),
		fmt.Sprintf("%.2f", report.PotentialSavings),
		fmt.Sprintf("%.1f", report.WastePercentage),
		fmt.Sprintf("Total Findings: %d", len(report.Findings)),
		"-",
		report.GeneratedAt.Format("2006-01-02 15:04:05"),
	}
	_ = writer.Write(summaryRow)
	_ = strconv.Itoa(len(report.Findings))

	latestPath := filepath.Join(outputDir, "report-latest.csv")
	_ = os.WriteFile(latestPath, []byte(""), 0600) // touch latest copy

	return filePath, nil
}
