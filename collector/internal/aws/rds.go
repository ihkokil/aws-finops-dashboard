package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

var rdsHourlyPrices = map[string]float64{
	"db.t3.micro":   0.017,
	"db.t3.small":   0.034,
	"db.t3.medium":  0.068,
	"db.t3.large":   0.136,
	"db.t3.xlarge":  0.272,
	"db.t4g.micro":  0.016,
	"db.t4g.small":  0.032,
	"db.t4g.medium": 0.064,
	"db.t4g.large":  0.128,
	"db.m5.large":   0.192,
	"db.m5.xlarge":  0.384,
	"db.m5.2xlarge": 0.768,
	"db.m6g.large":  0.154,
	"db.m6g.xlarge": 0.308,
	"db.r5.large":   0.240,
	"db.r5.xlarge":  0.480,
	"db.r6g.large":  0.208,
	"db.r6g.xlarge": 0.416,
}

// RDSAnalyzer scans RDS instances for idle connections and non-prod Multi-AZ setups.
type RDSAnalyzer struct {
	rdsClient *rds.Client
	cwClient  *cloudwatch.Client
	region    string
	accountID string
}

// NewRDSAnalyzer initializes an RDS analyzer instance.
func NewRDSAnalyzer(cfg aws.Config, region, accountID string) *RDSAnalyzer {
	return &RDSAnalyzer{
		rdsClient: rds.NewFromConfig(cfg),
		cwClient:  cloudwatch.NewFromConfig(cfg),
		region:    region,
		accountID: accountID,
	}
}

// FindIdleRDSInstances scans database instances for low connection activity.
func (r *RDSAnalyzer) FindIdleRDSInstances(ctx context.Context, days int) ([]models.Finding, error) {
	out, err := r.rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe RDS instances: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()
	startTime := now.AddDate(0, 0, -days)

	for _, db := range out.DBInstances {
		dbID := aws.ToString(db.DBInstanceIdentifier)
		dbClass := aws.ToString(db.DBInstanceClass)

		hourlyRate, ok := rdsHourlyPrices[dbClass]
		if !ok {
			hourlyRate = 0.20
		}
		monthlyCost := hourlyRate * 730.0
		if db.MultiAZ != nil && *db.MultiAZ {
			monthlyCost *= 2.0
		}

		avgConn, err := r.getRDSConnections(ctx, dbID, startTime, now)
		if err != nil {
			avgConn = 0.0 // Default if metric call fails
		}

		avgCPU, _ := r.getRDSCPU(ctx, dbID, startTime, now)

		var isIdle, isUnderutilized bool
		if avgConn < 1.0 {
			isIdle = true
		} else if avgConn < 5.0 && avgCPU < 10.0 {
			isUnderutilized = true
		}

		if !isIdle && !isUnderutilized {
			continue
		}

		tags := make(map[string]string)
		for _, t := range db.TagList {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}

		var category, recommendation string
		var wasteCost float64
		var remediation []string

		if isIdle {
			category = models.CategoryIdle
			wasteCost = monthlyCost
			recommendation = fmt.Sprintf("Idle RDS instance %s (%s) — avg connections %.1f over %d days. Consider Aurora Serverless v2 or stop instance outside business hours.", dbID, dbClass, avgConn, days)
			remediation = []string{
				fmt.Sprintf("aws rds stop-db-instance --db-instance-identifier %s --region %s", dbID, r.region),
				fmt.Sprintf("Consider migrating to Aurora Serverless v2 for automatic scaling down to 0.5 ACU"),
			}
		} else {
			category = models.CategoryRightsizing
			wasteCost = monthlyCost * 0.40
			recommendation = fmt.Sprintf("Underutilized RDS instance %s (%s) — avg connections %.1f, avg CPU %.1f%%. Downsize instance class.", dbID, dbClass, avgConn, avgCPU)
			remediation = []string{
				fmt.Sprintf("aws rds modify-db-instance --db-instance-identifier %s --db-instance-class <smaller-class> --apply-immediately", dbID),
			}
		}

		f := models.Finding{
			ID:                fmt.Sprintf("rds-idle-%s", dbID),
			Category:          category,
			Severity:          models.DetermineSeverity(wasteCost),
			ResourceType:      "rds",
			ResourceID:        dbID,
			ResourceName:      dbID,
			Region:            r.region,
			AccountID:         r.accountID,
			Tags:              tags,
			MonthlyWasteCost:  wasteCost,
			AnnualWasteCost:   wasteCost * 12.0,
			EstimatedSavings:  wasteCost,
			SavingsPercentage: (wasteCost / monthlyCost) * 100.0,
			Recommendation:    recommendation,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"avg_connections": avgConn,
				"avg_cpu":         avgCPU,
				"monthly_cost":    monthlyCost,
			},
		}

		findings = append(findings, f)
	}

	return findings, nil
}

// FindMultiAZCandidates identifies Multi-AZ RDS instances running in non-production environments.
func (r *RDSAnalyzer) FindMultiAZCandidates(ctx context.Context) ([]models.Finding, error) {
	out, err := r.rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe RDS instances: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()

	nonProdEnvs := map[string]bool{
		"dev":         true,
		"development": true,
		"stage":       true,
		"staging":     true,
		"test":        true,
		"testing":     true,
		"sandbox":     true,
		"qa":          true,
	}

	for _, db := range out.DBInstances {
		if db.MultiAZ == nil || !*db.MultiAZ {
			continue
		}

		dbID := aws.ToString(db.DBInstanceIdentifier)
		dbClass := aws.ToString(db.DBInstanceClass)

		isNonProd := false
		tags := make(map[string]string)
		for _, t := range db.TagList {
			k := strings.ToLower(aws.ToString(t.Key))
			v := strings.ToLower(aws.ToString(t.Value))
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)

			if (k == "env" || k == "environment" || k == "stage") && nonProdEnvs[v] {
				isNonProd = true
			}
		}

		// Check identifier name fallback if tags not found
		dbLower := strings.ToLower(dbID)
		for envKey := range nonProdEnvs {
			if strings.Contains(dbLower, envKey) {
				isNonProd = true
				break
			}
		}

		if !isNonProd {
			continue
		}

		hourlyRate, ok := rdsHourlyPrices[dbClass]
		if !ok {
			hourlyRate = 0.20
		}
		totalCost := hourlyRate * 730.0 * 2.0 // Multi-AZ double cost
		multiAZPremium := totalCost * 0.50     // 50% savings by switching to single-AZ

		rec := fmt.Sprintf("Non-prod Multi-AZ RDS instance %s (%s) — disable Multi-AZ for 50%% cost reduction ($%.2f/month savings)", dbID, dbClass, multiAZPremium)
		remediation := []string{
			fmt.Sprintf("aws rds modify-db-instance --db-instance-identifier %s --no-multi-az --apply-immediately", dbID),
		}

		f := models.Finding{
			ID:                fmt.Sprintf("rds-multiaz-%s", dbID),
			Category:          models.CategoryRightsizing,
			Severity:          models.DetermineSeverity(multiAZPremium),
			ResourceType:      "rds",
			ResourceID:        dbID,
			ResourceName:      dbID,
			Region:            r.region,
			AccountID:         r.accountID,
			Tags:              tags,
			MonthlyWasteCost:  multiAZPremium,
			AnnualWasteCost:   multiAZPremium * 12.0,
			EstimatedSavings:  multiAZPremium,
			SavingsPercentage: 50.0,
			Recommendation:    rec,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"monthly_cost":     totalCost,
				"multiaz_premium":  multiAZPremium,
			},
		}

		findings = append(findings, f)
	}

	return findings, nil
}

func (r *RDSAnalyzer) getRDSConnections(ctx context.Context, dbID string, start, end time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("DatabaseConnections"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("DBInstanceIdentifier"),
				Value: aws.String(dbID),
			},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	out, err := r.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 0, err
	}

	var sum float64
	for _, dp := range out.Datapoints {
		if dp.Average != nil {
			sum += *dp.Average
		}
	}
	return sum / float64(len(out.Datapoints)), nil
}

func (r *RDSAnalyzer) getRDSCPU(ctx context.Context, dbID string, start, end time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("DBInstanceIdentifier"),
				Value: aws.String(dbID),
			},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	out, err := r.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 0, err
	}

	var sum float64
	for _, dp := range out.Datapoints {
		if dp.Average != nil {
			sum += *dp.Average
		}
	}
	return sum / float64(len(out.Datapoints)), nil
}
