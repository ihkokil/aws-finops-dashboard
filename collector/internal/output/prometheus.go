package output

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

var (
	once sync.Once

	TotalSpendGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_total_monthly_spend_dollars",
			Help: "Total monthly AWS spend in USD",
		},
		[]string{"account_id", "region"},
	)

	MonthlyWasteGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_monthly_waste_dollars",
			Help: "Total identified monthly waste in USD by category",
		},
		[]string{"account_id", "region", "category"},
	)

	PotentialSavingsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_potential_savings_dollars",
			Help: "Realistic monthly potential savings in USD (70% of waste)",
		},
		[]string{"account_id", "region"},
	)

	WastePercentageGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_waste_percentage",
			Help: "Identified waste percentage of total monthly spend",
		},
		[]string{"account_id", "region"},
	)

	ResourceWasteGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_resource_waste_dollars",
			Help: "Monthly waste cost per identified resource",
		},
		[]string{"resource_type", "resource_id", "resource_name", "region", "severity", "category"},
	)

	SavingsPlanUtilizationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_savings_plan_utilization_percent",
			Help: "Savings Plan utilization percentage",
		},
		[]string{"account_id"},
	)

	RIUtilizationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_ri_utilization_percent",
			Help: "Reserved Instance utilization percentage by service",
		},
		[]string{"account_id", "service"},
	)

	RIUnusedHoursGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_ri_unused_hours",
			Help: "Unused Reserved Instance hours by service",
		},
		[]string{"account_id", "service"},
	)

	ForecastGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_finops_monthly_forecast_dollars",
			Help: "Predicted monthly spend forecast",
		},
		[]string{"account_id", "confidence_level"},
	)

	CollectorRunsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_finops_collector_runs_total",
			Help: "Total number of collector executions",
		},
		[]string{"status"},
	)

	FindingsTotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_finops_findings_total",
			Help: "Count of findings categorized by severity and category",
		},
		[]string{"severity", "category"},
	)

	CollectorDurationHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "aws_finops_collector_duration_seconds",
			Help:    "Execution duration of the FinOps collector in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 8),
		},
	)
)

func initMetrics() {
	once.Do(func() {
		prometheus.MustRegister(TotalSpendGauge)
		prometheus.MustRegister(MonthlyWasteGauge)
		prometheus.MustRegister(PotentialSavingsGauge)
		prometheus.MustRegister(WastePercentageGauge)
		prometheus.MustRegister(ResourceWasteGauge)
		prometheus.MustRegister(SavingsPlanUtilizationGauge)
		prometheus.MustRegister(RIUtilizationGauge)
		prometheus.MustRegister(RIUnusedHoursGauge)
		prometheus.MustRegister(ForecastGauge)
		prometheus.MustRegister(CollectorRunsCounter)
		prometheus.MustRegister(FindingsTotalCounter)
		prometheus.MustRegister(CollectorDurationHistogram)
	})
}

// UpdateMetrics updates all Prometheus metric vectors with the report data.
func UpdateMetrics(report *models.Report) {
	initMetrics()

	TotalSpendGauge.WithLabelValues(report.AccountID, report.Region).Set(report.TotalMonthlySpend)
	PotentialSavingsGauge.WithLabelValues(report.AccountID, report.Region).Set(report.PotentialSavings)
	WastePercentageGauge.WithLabelValues(report.AccountID, report.Region).Set(report.WastePercentage)

	// Reset category waste gauges
	for cat, waste := range report.FindingsByCategory {
		MonthlyWasteGauge.WithLabelValues(report.AccountID, report.Region, string(rune(cat[0]))).Set(float64(waste))
	}
	MonthlyWasteGauge.WithLabelValues(report.AccountID, report.Region, "total").Set(report.TotalMonthlyWaste)

	// Individual finding gauges
	ResourceWasteGauge.Reset()
	for _, f := range report.Findings {
		ResourceWasteGauge.WithLabelValues(
			f.ResourceType,
			f.ResourceID,
			f.ResourceName,
			f.Region,
			f.Severity,
			f.Category,
		).Set(f.MonthlyWasteCost)

		FindingsTotalCounter.WithLabelValues(f.Severity, f.Category).Inc()
	}

	// Commitments
	SavingsPlanUtilizationGauge.WithLabelValues(report.AccountID).Set(report.SavingsPlanUtilization.UtilizationPercentage)
	RIUtilizationGauge.WithLabelValues(report.AccountID, "ec2").Set(report.RIUtilization.UtilizationPercent)
	RIUnusedHoursGauge.WithLabelValues(report.AccountID, "ec2").Set(report.RIUtilization.UnusedHours)

	// Forecast
	ForecastGauge.WithLabelValues(report.AccountID, "mean").Set(report.Forecast.MeanValue)
	ForecastGauge.WithLabelValues(report.AccountID, "upper").Set(report.Forecast.PredictionIntervalUpper)
	ForecastGauge.WithLabelValues(report.AccountID, "lower").Set(report.Forecast.PredictionIntervalLower)
}

// ServeMetrics starts an HTTP server on the specified port serving /metrics.
func ServeMetrics(port int) error {
	initMetrics()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	return server.ListenAndServe()
}

// RecordRunDuration helper function for metric recording.
func RecordRunDuration(duration time.Duration, err error) {
	initMetrics()
	CollectorDurationHistogram.Observe(duration.Seconds())
	if err != nil {
		CollectorRunsCounter.WithLabelValues("failure").Inc()
	} else {
		CollectorRunsCounter.WithLabelValues("success").Inc()
	}
}
