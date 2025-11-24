package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

// NATAnalyzer evaluates NAT Gateway spend and identifies VPC Endpoint opportunities.
type NATAnalyzer struct {
	ec2Client *ec2.Client
	cwClient  *cloudwatch.Client
	region    string
	accountID string
}

// NewNATAnalyzer initializes a NAT Gateway analyzer.
func NewNATAnalyzer(cfg aws.Config, region, accountID string) *NATAnalyzer {
	return &NATAnalyzer{
		ec2Client: ec2.NewFromConfig(cfg),
		cwClient:  cloudwatch.NewFromConfig(cfg),
		region:    region,
		accountID: accountID,
	}
}

// FindExpensiveNATGateways analyzes NAT Gateway data throughput and checks VPC Endpoints.
func (n *NATAnalyzer) FindExpensiveNATGateways(ctx context.Context) ([]models.Finding, error) {
	input := &ec2.DescribeNatGatewaysInput{
		Filter: []ec2types.Filter{
			{
				Name:   aws.String("state"),
				Values: []string{"available"},
			},
		},
	}

	out, err := n.ec2Client.DescribeNatGateways(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe NAT Gateways: %w", err)
	}

	// Check VPC Endpoints in the region
	vpcEndpoints, _ := n.ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{})
	existingVpcEndpoints := make(map[string]bool)
	if vpcEndpoints != nil {
		for _, vpce := range vpcEndpoints.VpcEndpoints {
			svcName := aws.ToString(vpce.ServiceName)
			existingVpcEndpoints[svcName] = true
		}
	}

	var findings []models.Finding
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -30)

	for _, nat := range out.NatGateways {
		natID := aws.ToString(nat.NatGatewayId)
		vpcID := aws.ToString(nat.VpcId)

		bytesOut, _ := n.getNATBytesOut(ctx, natID, start, now)
		processedGB := bytesOut / (1024 * 1024 * 1024)

		// NAT Gateway cost = $0.045/hour (~$32.40/month) + $0.045 per GB processed
		baseMonthly := 32.40
		dataCost := processedGB * 0.045
		totalMonthlyCost := baseMonthly + dataCost

		tags := make(map[string]string)
		natName := natID
		for _, t := range nat.Tags {
			k := aws.ToString(t.Key)
			v := aws.ToString(t.Value)
			tags[k] = v
			if k == "Name" && v != "" {
				natName = v
			}
		}

		isNonProd := false
		nameLower := strings.ToLower(natName)
		if strings.Contains(nameLower, "dev") || strings.Contains(nameLower, "stage") || strings.Contains(nameLower, "test") {
			isNonProd = true
		}

		// Flag 1: High NAT Gateway Data Processing Cost -> Recommend S3 / ECR / Secrets Gateway Endpoints
		s3EndpointKey := fmt.Sprintf("com.amazonaws.%s.s3", n.region)
		ecrEndpointKey := fmt.Sprintf("com.amazonaws.%s.ecr.api", n.region)

		if totalMonthlyCost > 50.0 && (!existingVpcEndpoints[s3EndpointKey] || !existingVpcEndpoints[ecrEndpointKey]) {
			potentialEndpointSavings := totalMonthlyCost * 0.40 // ~40% savings off data transfer

			rec := fmt.Sprintf("NAT Gateway %s ($%.2f/month, %.1f GB processed) lacks S3/ECR Gateway Endpoints in VPC %s. Create free Gateway Endpoints to eliminate data processing fees.", natName, totalMonthlyCost, processedGB, vpcID)
			remediation := []string{
				fmt.Sprintf("aws ec2 create-vpc-endpoint --vpc-id %s --service-name com.amazonaws.%s.s3 --vpc-endpoint-type Gateway --region %s", vpcID, n.region, n.region),
				fmt.Sprintf("aws ec2 create-vpc-endpoint --vpc-id %s --service-name com.amazonaws.%s.ecr.dkr --vpc-endpoint-type Interface --region %s", vpcID, n.region, n.region),
			}

			f := models.Finding{
				ID:                fmt.Sprintf("nat-endpoint-%s", natID),
				Category:          models.CategoryNetwork,
				Severity:          models.DetermineSeverity(potentialEndpointSavings),
				ResourceType:      "nat",
				ResourceID:        natID,
				ResourceName:      natName,
				Region:            n.region,
				AccountID:         n.accountID,
				Tags:              tags,
				MonthlyWasteCost:  potentialEndpointSavings,
				AnnualWasteCost:   potentialEndpointSavings * 12.0,
				EstimatedSavings:  potentialEndpointSavings,
				SavingsPercentage: 40.0,
				Recommendation:    rec,
				RemediationSteps:  remediation,
				Confidence:        "high",
				DetectedAt:        now,
				Metrics: map[string]float64{
					"processed_gb":       processedGB,
					"total_monthly_cost": totalMonthlyCost,
					"estimated_savings": potentialEndpointSavings,
				},
			}

			findings = append(findings, f)
		} else if isNonProd && totalMonthlyCost > 30.0 {
			// Flag 2: 24/7 Non-Prod NAT Gateway
			scheduleSavings := baseMonthly * 0.50 // 50% savings by shutting down outside business hours

			rec := fmt.Sprintf("Non-prod NAT Gateway %s in VPC %s runs 24/7. Schedule off-hours shutdown to save $%.2f/month.", natName, vpcID, scheduleSavings)
			remediation := []string{
				fmt.Sprintf("Automate NAT Gateway teardown/recreation via Lambda outside business hours in VPC %s", vpcID),
			}

			f := models.Finding{
				ID:                fmt.Sprintf("nat-schedule-%s", natID),
				Category:          models.CategoryNetwork,
				Severity:          models.DetermineSeverity(scheduleSavings),
				ResourceType:      "nat",
				ResourceID:        natID,
				ResourceName:      natName,
				Region:            n.region,
				AccountID:         n.accountID,
				Tags:              tags,
				MonthlyWasteCost:  scheduleSavings,
				AnnualWasteCost:   scheduleSavings * 12.0,
				EstimatedSavings:  scheduleSavings,
				SavingsPercentage: 50.0,
				Recommendation:    rec,
				RemediationSteps:  remediation,
				Confidence:        "medium",
				DetectedAt:        now,
				Metrics: map[string]float64{
					"processed_gb":       processedGB,
					"total_monthly_cost": totalMonthlyCost,
					"estimated_savings": scheduleSavings,
				},
			}

			findings = append(findings, f)
		}
	}

	return findings, nil
}

func (n *NATAnalyzer) getNATBytesOut(ctx context.Context, natID string, start, end time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/NATGateway"),
		MetricName: aws.String("BytesOutToDestination"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("NatGatewayId"), Value: aws.String(natID)},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
	}

	out, err := n.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 50.0 * 1024 * 1024 * 1024, nil // Fallback estimate (50 GB) if CW metric empty
	}

	var sum float64
	for _, dp := range out.Datapoints {
		if dp.Sum != nil {
			sum += *dp.Sum
		}
	}

	return sum, nil
}
