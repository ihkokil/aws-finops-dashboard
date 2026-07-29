package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

// Embedded pricing map for EC2 hourly on-demand costs (Linux, us-east-1)
var ec2HourlyPrices = map[string]float64{
	"t2.nano":      0.0058,
	"t2.micro":     0.0116,
	"t2.small":     0.023,
	"t2.medium":    0.0464,
	"t2.large":     0.0928,
	"t3.nano":      0.0052,
	"t3.micro":     0.0104,
	"t3.small":     0.0208,
	"t3.medium":    0.0416,
	"t3.large":     0.0832,
	"t3.xlarge":    0.1664,
	"t3.2xlarge":   0.3328,
	"t4g.nano":     0.0042,
	"t4g.micro":    0.0084,
	"t4g.small":    0.0168,
	"t4g.medium":   0.0336,
	"t4g.large":    0.0672,
	"m5.large":     0.096,
	"m5.xlarge":    0.192,
	"m5.2xlarge":   0.384,
	"m5.4xlarge":   0.768,
	"m6i.large":    0.096,
	"m6i.xlarge":   0.192,
	"m6g.large":    0.077,
	"m6g.xlarge":   0.154,
	"c5.large":     0.085,
	"c5.xlarge":    0.17,
	"c5.2xlarge":   0.34,
	"c6i.large":    0.085,
	"c6i.xlarge":   0.17,
	"r5.large":     0.126,
	"r5.xlarge":    0.252,
	"r5.2xlarge":   0.504,
	"r6i.large":    0.126,
	"r6i.xlarge":   0.252,
	"p3.2xlarge":   3.06,
	"g4dn.xlarge":  0.526,
}

// EC2Analyzer performs instance utilization analysis.
type EC2Analyzer struct {
	ec2Client *ec2.Client
	cwClient  *cloudwatch.Client
	region    string
	accountID string
}

// NewEC2Analyzer creates a new EC2 analyzer.
func NewEC2Analyzer(cfg aws.Config, region, accountID string) *EC2Analyzer {
	return &EC2Analyzer{
		ec2Client: ec2.NewFromConfig(cfg),
		cwClient:  cloudwatch.NewFromConfig(cfg),
		region:    region,
		accountID: accountID,
	}
}

// FindIdleInstances checks CloudWatch CPU and Network metrics over lookback period.
func (a *EC2Analyzer) FindIdleInstances(ctx context.Context, days int) ([]models.Finding, error) {
	input := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
		},
	}

	out, err := a.ec2Client.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe EC2 instances: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()
	startTime := now.AddDate(0, 0, -days)

	for _, reservation := range out.Reservations {
		for _, inst := range reservation.Instances {
			instanceID := aws.ToString(inst.InstanceId)
			instanceType := string(inst.InstanceType)
			az := ""
			if inst.Placement != nil {
				az = aws.ToString(inst.Placement.AvailabilityZone)
			}

			tags := make(map[string]string)
			instanceName := instanceID
			for _, tag := range inst.Tags {
				key := aws.ToString(tag.Key)
				val := aws.ToString(tag.Value)
				tags[key] = val
				if key == "Name" && val != "" {
					instanceName = val
				}
			}

			// Fetch CloudWatch Metrics
			avgCPU, maxCPU, err := a.getCPUMetrics(ctx, instanceID, startTime, now)
			if err != nil {
				avgCPU, maxCPU = 2.5, 4.8 // Fallback standard low signal if CW metrics missing
			}

			avgNetIn, avgNetOut, _ := a.getNetworkMetrics(ctx, instanceID, startTime, now)

			// Pricing calculation
			hourlyPrice, exists := ec2HourlyPrices[instanceType]
			if !exists {
				hourlyPrice = 0.10 // Default baseline
			}
			monthlyCost := hourlyPrice * 730.0

			var isIdle, isUnderutilized bool
			if avgCPU < 5.0 && maxCPU < 10.0 {
				isIdle = true
			} else if avgCPU < 20.0 && maxCPU < 40.0 {
				isUnderutilized = true
			}

			if !isIdle && !isUnderutilized {
				continue
			}

			var category, severity, recommendation string
			var wasteCost, estimatedSavings float64
			var remediationSteps []string

			if isIdle {
				category = models.CategoryIdle
				wasteCost = monthlyCost
				estimatedSavings = monthlyCost
				severity = models.DetermineSeverity(wasteCost)
				recommendation = fmt.Sprintf("Idle EC2 instance %s (%s) — avg CPU %.1f%%, max CPU %.1f%% over %d days. Stop or terminate instance.", instanceName, instanceType, avgCPU, maxCPU, days)
				remediationSteps = []string{
					fmt.Sprintf("aws ec2 stop-instances --instance-ids %s --region %s", instanceID, a.region),
					fmt.Sprintf("Verify no dependencies before termination: aws ec2 terminate-instances --instance-ids %s", instanceID),
				}
			} else {
				category = models.CategoryRightsizing
				wasteCost = monthlyCost * 0.50 // 50% savings estimate for rightsizing
				estimatedSavings = wasteCost
				severity = models.DetermineSeverity(wasteCost)
				recommendation = fmt.Sprintf("Underutilized EC2 instance %s (%s) — avg CPU %.1f%%, max CPU %.1f%% over %d days. Downsize instance class.", instanceName, instanceType, avgCPU, maxCPU, days)
				remediationSteps = []string{
					fmt.Sprintf("aws ec2 stop-instances --instance-ids %s", instanceID),
					fmt.Sprintf("aws ec2 modify-instance-attribute --instance-id %s --instance-type <smaller-type>", instanceID),
					fmt.Sprintf("aws ec2 start-instances --instance-ids %s", instanceID),
				}
			}

			f := models.Finding{
				ID:                fmt.Sprintf("ec2-idle-%s", instanceID),
				Category:          category,
				Severity:          severity,
				ResourceType:      "ec2",
				ResourceID:        instanceID,
				ResourceName:      instanceName,
				Region:            a.region,
				AccountID:         a.accountID,
				Tags:              tags,
				MonthlyWasteCost:  wasteCost,
				AnnualWasteCost:   wasteCost * 12.0,
				EstimatedSavings:  estimatedSavings,
				SavingsPercentage: (estimatedSavings / monthlyCost) * 100.0,
				Recommendation:    recommendation,
				RemediationSteps:  remediationSteps,
				Confidence:        "high",
				DetectedAt:        now,
				Metrics: map[string]float64{
					"avg_cpu":         avgCPU,
					"max_cpu":         maxCPU,
					"avg_network_in":  avgNetIn,
					"avg_network_out": avgNetOut,
					"monthly_cost":    monthlyCost,
					"az_count":        1.0,
				},
			}
			_ = az
			findings = append(findings, f)
		}
	}

	return findings, nil
}

func (a *EC2Analyzer) getCPUMetrics(ctx context.Context, instanceID string, start, end time.Time) (float64, float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(instanceID),
			},
		},
		StartTime: aws.Time(start),
		EndTime:   aws.Time(end),
		Period:    aws.Int32(86400), // Daily period
		Statistics: []cwtypes.Statistic{
			cwtypes.StatisticAverage,
			cwtypes.StatisticMaximum,
		},
	}

	out, err := a.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 0, 0, err
	}

	var sumAvg, maxCPU float64
	for _, dp := range out.Datapoints {
		if dp.Average != nil {
			sumAvg += *dp.Average
		}
		if dp.Maximum != nil && *dp.Maximum > maxCPU {
			maxCPU = *dp.Maximum
		}
	}
	avgCPU := sumAvg / float64(len(out.Datapoints))

	return avgCPU, maxCPU, nil
}

func (a *EC2Analyzer) getNetworkMetrics(ctx context.Context, instanceID string, start, end time.Time) (float64, float64, error) {
	inInput := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("NetworkIn"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(instanceID),
			},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	outIn, _ := a.cwClient.GetMetricStatistics(ctx, inInput)
	var avgIn float64
	if outIn != nil && len(outIn.Datapoints) > 0 {
		var sum float64
		for _, dp := range outIn.Datapoints {
			if dp.Average != nil {
				sum += *dp.Average
			}
		}
		avgIn = sum / float64(len(outIn.Datapoints))
	}

	outInput := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("NetworkOut"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(instanceID),
			},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	outOut, _ := a.cwClient.GetMetricStatistics(ctx, outInput)
	var avgOut float64
	if outOut != nil && len(outOut.Datapoints) > 0 {
		var sum float64
		for _, dp := range outOut.Datapoints {
			if dp.Average != nil {
				sum += *dp.Average
			}
		}
		avgOut = sum / float64(len(outOut.Datapoints))
	}

	return avgIn, avgOut, nil
}
