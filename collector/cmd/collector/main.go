package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/sync/errgroup"

	finopsaws "github.com/aws-finops-dashboard/collector/internal/aws"
	"github.com/aws-finops-dashboard/collector/internal/config"
	"github.com/aws-finops-dashboard/collector/internal/models"
	"github.com/aws-finops-dashboard/collector/internal/output"
)

func main() {
	startTime := time.Now()
	cfg := config.LoadConfig()

	log.Println("[INFO] Initializing AWS FinOps Collector...")
	ctx := context.Background()

	// Load AWS SDK Configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Regions[0]))
	if err != nil {
		log.Fatalf("[FATAL] Failed to load AWS SDK config: %v", err)
	}

	// Validate Credentials via STS GetCallerIdentity
	stsClient := sts.NewFromConfig(awsCfg)
	callerID, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	var accountID string
	if err != nil {
		log.Printf("[WARN] Unable to verify STS caller identity: %v. Using default fallback account.", err)
		accountID = "123456789012"
	} else {
		accountID = *callerID.Account
		log.Printf("[INFO] Authenticated AWS Account: %s | Primary Region: %s", accountID, cfg.Regions[0])
	}

	report := &models.Report{
		GeneratedAt: time.Now().UTC(),
		AccountID:   accountID,
		Region:      cfg.Regions[0],
		PeriodDays:  cfg.Days,
		Findings:    make([]models.Finding, 0),
	}

	// Concurrent Collection Execution via errgroup
	g, gCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex

	ceClient := finopsaws.NewCostExplorerClient(awsCfg)
	ec2Analyzer := finopsaws.NewEC2Analyzer(awsCfg, cfg.Regions[0], accountID)
	rightsizingAnalyzer := finopsaws.NewRightsizingAnalyzer(awsCfg, cfg.Regions[0], accountID)
	ebsAnalyzer := finopsaws.NewEBSAnalyzer(awsCfg, cfg.Regions[0], accountID)
	rdsAnalyzer := finopsaws.NewRDSAnalyzer(awsCfg, cfg.Regions[0], accountID)
	s3Analyzer := finopsaws.NewS3Analyzer(awsCfg, accountID)
	elbAnalyzer := finopsaws.NewELBAnalyzer(awsCfg, cfg.Regions[0], accountID)
	natAnalyzer := finopsaws.NewNATAnalyzer(awsCfg, cfg.Regions[0], accountID)

	// 1. Cost Explorer Spend & Trends
	g.Go(func() error {
		now := time.Now().UTC()
		endStr := now.Format("2006-01-02")
		startStr := now.AddDate(0, -1, 0).Format("2006-01-02")

		services, err := ceClient.GetMonthlyCostByService(gCtx, startStr, endStr)
		if err == nil {
			var total float64
			for _, s := range services {
				total += s.UnblendedCost
			}
			mu.Lock()
			report.TopServices = services
			report.TotalMonthlySpend = total
			mu.Unlock()
		}

		daily, _ := ceClient.GetDailyCostLast90Days(gCtx)
		mu.Lock()
		report.DailyCosts = daily
		mu.Unlock()

		sp, _ := ceClient.GetSavingsPlanUtilization(gCtx)
		ri, _ := ceClient.GetReservedInstanceUtilization(gCtx)
		forecast, _ := ceClient.GetCostForecast(gCtx, endStr, now.AddDate(0, 1, 0).Format("2006-01-02"))

		mu.Lock()
		report.SavingsPlanUtilization = sp
		report.RIUtilization = ri
		report.Forecast = forecast
		mu.Unlock()

		return nil
	})

	// 2. EC2 Idle & Underutilized
	g.Go(func() error {
		findings, err := ec2Analyzer.FindIdleInstances(gCtx, cfg.Days)
		if err == nil {
			mu.Lock()
			report.Findings = append(report.Findings, findings...)
			mu.Unlock()
		}
		return nil
	})

	// 3. EC2 Rightsizing
	g.Go(func() error {
		findings, err := rightsizingAnalyzer.FindRightsizingOpportunities(gCtx)
		if err == nil {
			mu.Lock()
			report.Findings = append(report.Findings, findings...)
			mu.Unlock()
		}
		return nil
	})

	// 4. EBS Unattached Volumes & Old Snapshots
	g.Go(func() error {
		vols, err1 := ebsAnalyzer.FindUnattachedVolumes(gCtx)
		snaps, err2 := ebsAnalyzer.FindOldSnapshots(gCtx)
		mu.Lock()
		if err1 == nil {
			report.Findings = append(report.Findings, vols...)
		}
		if err2 == nil {
			report.Findings = append(report.Findings, snaps...)
		}
		mu.Unlock()
		return nil
	})

	// 5. RDS Idle & Non-Prod Multi-AZ
	g.Go(func() error {
		idles, err1 := rdsAnalyzer.FindIdleRDSInstances(gCtx, cfg.Days)
		multiaz, err2 := rdsAnalyzer.FindMultiAZCandidates(gCtx)
		mu.Lock()
		if err1 == nil {
			report.Findings = append(report.Findings, idles...)
		}
		if err2 == nil {
			report.Findings = append(report.Findings, multiaz...)
		}
		mu.Unlock()
		return nil
	})

	// 6. S3 Bucket Lifecycle Policy Gaps
	g.Go(func() error {
		findings, err := s3Analyzer.FindBucketsWithoutLifecycle(gCtx)
		if err == nil {
			mu.Lock()
			report.Findings = append(report.Findings, findings...)
			mu.Unlock()
		}
		return nil
	})

	// 7. ELB Idle Load Balancers
	g.Go(func() error {
		findings, err := elbAnalyzer.FindIdleLoadBalancers(gCtx, cfg.Days)
		if err == nil {
			mu.Lock()
			report.Findings = append(report.Findings, findings...)
			mu.Unlock()
		}
		return nil
	})

	// 8. NAT Gateway Cost & Endpoint Analysis
	g.Go(func() error {
		findings, err := natAnalyzer.FindExpensiveNATGateways(gCtx)
		if err == nil {
			mu.Lock()
			report.Findings = append(report.Findings, findings...)
			mu.Unlock()
		}
		return nil
	})

	// Wait for all collectors
	if err := g.Wait(); err != nil {
		log.Printf("[WARN] Collector error during execution: %v", err)
	}

	// Calculate summary stats and filter low-value findings
	report.CalculateSummaryStats(cfg.MinSavings)

	duration := time.Since(startTime)
	output.RecordRunDuration(duration, nil)
	output.UpdateMetrics(report)

	// Print Summary to Stdout
	topSavingDesc := "None"
	if len(report.Findings) > 0 {
		topSavingDesc = fmt.Sprintf("%s (%s: $%.2f/month)", report.Findings[0].Recommendation, report.Findings[0].ResourceType, report.Findings[0].MonthlyWasteCost)
	}
	summaryLine := fmt.Sprintf("Found %d findings | Total waste: $%.0f/month | Top saving: %s", len(report.Findings), report.TotalMonthlyWaste, topSavingDesc)
	fmt.Println(summaryLine)

	if !cfg.DryRun {
		// Export output files based on format
		if cfg.OutputFormat == "json" || cfg.OutputFormat == "all" {
			path, err := output.WriteJSON(report, cfg.OutputDir)
			if err != nil {
				log.Printf("[ERROR] JSON write failed: %v", err)
			} else {
				log.Printf("[INFO] Saved JSON report: %s", path)
			}
		}

		if cfg.OutputFormat == "csv" || cfg.OutputFormat == "all" {
			path, err := output.WriteCSV(report, cfg.OutputDir)
			if err != nil {
				log.Printf("[ERROR] CSV write failed: %v", err)
			} else {
				log.Printf("[INFO] Saved CSV report: %s", path)
			}
		}

		if cfg.OutputFormat == "markdown" || cfg.OutputFormat == "all" {
			path, err := output.WriteMarkdown(report, cfg.OutputDir)
			if err != nil {
				log.Printf("[ERROR] Markdown write failed: %v", err)
			} else {
				log.Printf("[INFO] Saved Markdown report: %s", path)
			}
		}
	}

	// If serve mode enabled, keep running Prometheus metrics HTTP server
	if cfg.Serve {
		log.Printf("[INFO] Starting Prometheus metrics server on :%d/metrics...", cfg.ServePort)
		if err := output.ServeMetrics(cfg.ServePort); err != nil {
			log.Fatalf("[FATAL] Metrics server error: %v", err)
		}
	}

	os.Exit(0)
}
