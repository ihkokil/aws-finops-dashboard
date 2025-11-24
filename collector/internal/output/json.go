package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws-finops-dashboard/collector/internal/models"
)

// WriteJSON exports the full report struct to a formatted JSON file.
func WriteJSON(report *models.Report, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := fmt.Sprintf("report-%s.json", report.GeneratedAt.Format("2006-01-02"))
	filePath := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON report: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write JSON report file: %w", err)
	}

	// Also update latest link / file
	latestPath := filepath.Join(outputDir, "report-latest.json")
	_ = os.WriteFile(latestPath, data, 0600)

	return filePath, nil
}
