package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
)

// Config encapsulates runtime collector parameters from CLI flags and environment variables.
type Config struct {
	OutputFormat string   // "json", "csv", "markdown", "prometheus", "all"
	OutputDir    string   // Output folder path
	Regions      []string // Target AWS regions
	Days         int      // Lookback period for CloudWatch / CE
	Serve        bool     // HTTP server mode for /metrics
	ServePort    int      // HTTP server port
	MinSavings   float64  // Threshold for filtering findings
	DryRun       bool     // Print summary without writing files
}

// LoadConfig parses environment variables and command line flags.
func LoadConfig() *Config {
	cfg := &Config{}

	var regionsStr string
	defaultOutputFormat := getEnv("OUTPUT_FORMAT", "all")
	defaultOutputDir := getEnv("OUTPUT_DIR", "./reports")
	defaultRegions := getEnv("AWS_REGION", "us-east-1")
	defaultDaysStr := getEnv("LOOKBACK_DAYS", "14")
	defaultServeStr := getEnv("SERVE", "false")
	defaultPortStr := getEnv("SERVE_PORT", "8080")
	defaultMinSavingsStr := getEnv("MIN_SAVINGS", "5.0")
	defaultDryRunStr := getEnv("DRY_RUN", "false")

	flag.StringVar(&cfg.OutputFormat, "output-format", defaultOutputFormat, "Output format: json|csv|markdown|prometheus|all")
	flag.StringVar(&cfg.OutputDir, "output-dir", defaultOutputDir, "Directory path to save reports")
	flag.StringVar(&regionsStr, "regions", defaultRegions, "Comma-separated list of AWS regions")

	defaultDays, _ := strconv.Atoi(defaultDaysStr)
	flag.IntVar(&cfg.Days, "days", defaultDays, "Lookback period in days for metric evaluation")

	defaultServe, _ := strconv.ParseBool(defaultServeStr)
	flag.BoolVar(&cfg.Serve, "serve", defaultServe, "Run as HTTP server exposing Prometheus /metrics endpoint")

	defaultPort, _ := strconv.Atoi(defaultPortStr)
	flag.IntVar(&cfg.ServePort, "serve-port", defaultPort, "Port for HTTP metrics server")

	defaultMinSavings, _ := strconv.ParseFloat(defaultMinSavingsStr, 64)
	flag.Float64Var(&cfg.MinSavings, "min-savings", defaultMinSavings, "Minimum monthly savings ($) required to include a finding")

	defaultDryRun, _ := strconv.ParseBool(defaultDryRunStr)
	flag.BoolVar(&cfg.DryRun, "dry-run", defaultDryRun, "Collect metrics and print report summary without writing output files")

	flag.Parse()

	// Parse regions list
	rawRegions := strings.Split(regionsStr, ",")
	for _, r := range rawRegions {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			cfg.Regions = append(cfg.Regions, trimmed)
		}
	}
	if len(cfg.Regions) == 0 {
		cfg.Regions = []string{"us-east-1"}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
