package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

// ELBAnalyzer identifies unused ALBs and NLBs.
type ELBAnalyzer struct {
	elbClient *elasticloadbalancingv2.Client
	cwClient  *cloudwatch.Client
	region    string
	accountID string
}

// NewELBAnalyzer creates an ELB analyzer instance.
func NewELBAnalyzer(cfg aws.Config, region, accountID string) *ELBAnalyzer {
	return &ELBAnalyzer{
		elbClient: elasticloadbalancingv2.NewFromConfig(cfg),
		cwClient:  cloudwatch.NewFromConfig(cfg),
		region:    region,
		accountID: accountID,
	}
}

// FindIdleLoadBalancers checks CloudWatch metrics for low or zero traffic across ALBs/NLBs.
func (e *ELBAnalyzer) FindIdleLoadBalancers(ctx context.Context, days int) ([]models.Finding, error) {
	out, err := e.elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe load balancers: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days)

	for _, lb := range out.LoadBalancers {
		arn := aws.ToString(lb.LoadBalancerArn)
		name := aws.ToString(lb.LoadBalancerName)
		lbType := string(lb.Type)

		// Parse CloudWatch Dimension suffix from ARN (app/lb-name/123456789)
		arnSuffix := extractLBSuffix(arn)

		var isIdle bool
		var metricVal float64
		baseMonthlyCost := 16.20 // Base price per ALB/NLB per month ($0.0225/hr)

		if lbType == "application" || lbType == string(elbtypes.LoadBalancerTypeEnumApplication) {
			reqCount, _ := e.getALBRequests(ctx, arnSuffix, start, now)
			metricVal = reqCount
			if reqCount < 100.0 { // Less than 100 requests/day avg
				isIdle = true
			}
		} else if lbType == "network" || lbType == string(elbtypes.LoadBalancerTypeEnumNetwork) {
			bytesProcessed, _ := e.getNLBBytes(ctx, arnSuffix, start, now)
			metricVal = bytesProcessed
			if bytesProcessed < (1024 * 1024) { // Less than 1MB/day
				isIdle = true
			}
		}

		if !isIdle {
			continue
		}

		rec := fmt.Sprintf("Idle Load Balancer %s (%s) — low traffic (%.0f avg activity) over %d days. Delete unneeded load balancer.", name, lbType, metricVal, days)
		remediation := []string{
			fmt.Sprintf("aws elbv2 delete-load-balancer --load-balancer-arn %s --region %s", arn, e.region),
		}

		f := models.Finding{
			ID:                fmt.Sprintf("elb-idle-%s", name),
			Category:          models.CategoryIdle,
			Severity:          models.DetermineSeverity(baseMonthlyCost),
			ResourceType:      "elb",
			ResourceID:        arn,
			ResourceName:      name,
			Region:            e.region,
			AccountID:         e.accountID,
			Tags:              map[string]string{"Type": lbType},
			MonthlyWasteCost:  baseMonthlyCost,
			AnnualWasteCost:   baseMonthlyCost * 12.0,
			EstimatedSavings:  baseMonthlyCost,
			SavingsPercentage: 100.0,
			Recommendation:    rec,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"traffic_metric": metricVal,
				"monthly_cost":   baseMonthlyCost,
			},
		}

		findings = append(findings, f)
	}

	return findings, nil
}

func (e *ELBAnalyzer) getALBRequests(ctx context.Context, arnSuffix string, start, end time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ApplicationELB"),
		MetricName: aws.String("RequestCount"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("LoadBalancer"), Value: aws.String(arnSuffix)},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
	}

	out, err := e.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 0, err
	}

	var total float64
	for _, dp := range out.Datapoints {
		if dp.Sum != nil {
			total += *dp.Sum
		}
	}
	return total / float64(len(out.Datapoints)), nil
}

func (e *ELBAnalyzer) getNLBBytes(ctx context.Context, arnSuffix string, start, end time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/NetworkELB"),
		MetricName: aws.String("ProcessedBytes"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("LoadBalancer"), Value: aws.String(arnSuffix)},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
	}

	out, err := e.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 0, err
	}

	var total float64
	for _, dp := range out.Datapoints {
		if dp.Sum != nil {
			total += *dp.Sum
		}
	}
	return total / float64(len(out.Datapoints)), nil
}

func extractLBSuffix(arn string) string {
	parts := strings.Split(arn, ":loadbalancer/")
	if len(parts) == 2 {
		return parts[1]
	}
	return arn
}
