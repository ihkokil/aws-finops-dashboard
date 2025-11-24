package aws

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

// RightsizingAnalyzer handles EC2 rightsizing recommendations via Cost Explorer.
type RightsizingAnalyzer struct {
	ceClient *costexplorer.Client
	region   string
	account  string
}

// NewRightsizingAnalyzer creates a rightsizing analyzer instance.
func NewRightsizingAnalyzer(cfg aws.Config, region, account string) *RightsizingAnalyzer {
	return &RightsizingAnalyzer{
		ceClient: costexplorer.NewFromConfig(cfg),
		region:   region,
		account:  account,
	}
}

// FindRightsizingOpportunities queries AWS Cost Explorer GetRightsizingRecommendation API.
func (r *RightsizingAnalyzer) FindRightsizingOpportunities(ctx context.Context) ([]models.Finding, error) {
	input := &costexplorer.GetRightsizingRecommendationInput{
		Service: aws.String("AmazonEC2"),
		Configuration: &cetypes.RightsizingRecommendationConfiguration{
			RecommendationTarget: cetypes.RecommendationTargetSameInstanceFamily,
			BenefitsConsidered:   true,
		},
	}

	out, err := r.ceClient.GetRightsizingRecommendation(ctx, input)
	var findings []models.Finding
	now := time.Now().UTC()

	if err != nil {
		// Return empty findings if Rightsizing API is not enabled or fails
		return findings, nil
	}

	for _, rec := range out.RightsizingRecommendations {
		if rec.RightsizingType != cetypes.RightsizingTypeModify && rec.RightsizingType != cetypes.RightsizingTypeTerminate {
			continue
		}

		resourceID := "unknown"
		currentType := "unknown"
		resourceName := "unknown"
		var currentCost float64

		if rec.CurrentInstance != nil {
			if rec.CurrentInstance.ResourceId != nil {
				resourceID = *rec.CurrentInstance.ResourceId
			}
			if rec.CurrentInstance.MonthlyCost != nil {
				currentCost, _ = strconv.ParseFloat(*rec.CurrentInstance.MonthlyCost, 64)
			}
			if rec.CurrentInstance.ResourceDetails != nil && rec.CurrentInstance.ResourceDetails.EC2ResourceDetails != nil {
				if rec.CurrentInstance.ResourceDetails.EC2ResourceDetails.InstanceType != nil {
					currentType = *rec.CurrentInstance.ResourceDetails.EC2ResourceDetails.InstanceType
				}
			}
			if rec.CurrentInstance.Tags != nil {
				for _, t := range rec.CurrentInstance.Tags {
					if aws.ToString(t.Key) == "Name" && len(t.Values) > 0 {
						resourceName = t.Values[0]
					}
				}
			}
		}

		if resourceName == "unknown" {
			resourceName = resourceID
		}

		var recommendedType string
		var estimatedSavings float64
		var projectedCost float64

		if rec.ModifyRecommendationDetail != nil && len(rec.ModifyRecommendationDetail.TargetInstances) > 0 {
			target := rec.ModifyRecommendationDetail.TargetInstances[0]
			if target.ResourceDetails != nil && target.ResourceDetails.EC2ResourceDetails != nil {
				if target.ResourceDetails.EC2ResourceDetails.InstanceType != nil {
					recommendedType = *target.ResourceDetails.EC2ResourceDetails.InstanceType
				}
			}
			if target.EstimatedMonthlySavings != nil {
				estimatedSavings, _ = strconv.ParseFloat(*target.EstimatedMonthlySavings, 64)
			}
			if target.EstimatedMonthlyCost != nil {
				projectedCost, _ = strconv.ParseFloat(*target.EstimatedMonthlyCost, 64)
			}
		} else if rec.TerminateRecommendationDetail != nil {
			recommendedType = "Terminate"
			if rec.TerminateRecommendationDetail.EstimatedMonthlySavings != nil {
				estimatedSavings, _ = strconv.ParseFloat(*rec.TerminateRecommendationDetail.EstimatedMonthlySavings, 64)
			}
			projectedCost = 0.0
		}

		if estimatedSavings <= 0 {
			continue
		}

		savingsPct := 0.0
		if currentCost > 0 {
			savingsPct = (estimatedSavings / currentCost) * 100.0
		}

		recText := fmt.Sprintf("Rightsizing: Change EC2 %s from %s to %s — save $%.2f/month", resourceName, currentType, recommendedType, estimatedSavings)
		remediation := []string{
			fmt.Sprintf("aws ec2 stop-instances --instance-ids %s", resourceID),
			fmt.Sprintf("aws ec2 modify-instance-attribute --instance-id %s --instance-type %s", resourceID, recommendedType),
			fmt.Sprintf("aws ec2 start-instances --instance-ids %s", resourceID),
		}

		f := models.Finding{
			ID:                fmt.Sprintf("rightsizing-ec2-%s", resourceID),
			Category:          models.CategoryRightsizing,
			Severity:          models.DetermineSeverity(estimatedSavings),
			ResourceType:      "ec2",
			ResourceID:        resourceID,
			ResourceName:      resourceName,
			Region:            r.region,
			AccountID:         r.account,
			Tags:              map[string]string{"CurrentType": currentType, "RecommendedType": recommendedType},
			MonthlyWasteCost:  estimatedSavings,
			AnnualWasteCost:   estimatedSavings * 12.0,
			EstimatedSavings:  estimatedSavings,
			SavingsPercentage: savingsPct,
			Recommendation:    recText,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"current_monthly_cost":   currentCost,
				"projected_monthly_cost": projectedCost,
				"estimated_savings":     estimatedSavings,
			},
		}

		findings = append(findings, f)
	}

	return findings, nil
}
