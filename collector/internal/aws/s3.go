package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

// S3Analyzer identifies storage cost waste and lifecycle configuration gaps.
type S3Analyzer struct {
	s3Client *s3.Client
	cwClient *cloudwatch.Client
	account  string
}

// NewS3Analyzer creates a new S3 analyzer instance.
func NewS3Analyzer(cfg aws.Config, account string) *S3Analyzer {
	return &S3Analyzer{
		s3Client: s3.NewFromConfig(cfg),
		cwClient: cloudwatch.NewFromConfig(cfg),
		account:  account,
	}
}

// FindBucketsWithoutLifecycle inspects S3 buckets for missing lifecycle policies.
func (s *S3Analyzer) FindBucketsWithoutLifecycle(ctx context.Context) ([]models.Finding, error) {
	out, err := s.s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()

	for _, bucket := range out.Buckets {
		bucketName := aws.ToString(bucket.Name)

		// Check Lifecycle Configuration
		_, lifecycleErr := s.s3Client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{
			Bucket: aws.String(bucketName),
		})

		hasLifecycle := lifecycleErr == nil

		// Fetch Bucket Size metric from CloudWatch
		sizeGB := s.getBucketSizeGB(ctx, bucketName)

		// Standard S3 pricing ($0.023/GB)
		monthlyCost := sizeGB * 0.023

		if !hasLifecycle && sizeGB > 1.0 && monthlyCost > 10.0 {
			estimatedSavings := monthlyCost * 0.30 // ~30% savings with Intelligent-Tiering

			rec := fmt.Sprintf("S3 Bucket '%s' (%.1f GB, $%.2f/month) missing lifecycle policy. Add Intelligent-Tiering transition for estimated 30%% savings ($%.2f/month).", bucketName, sizeGB, monthlyCost, estimatedSavings)
			remediation := []string{
				fmt.Sprintf("aws s3api put-bucket-lifecycle-configuration --bucket %s --lifecycle-configuration '{\"Rules\":[{\"ID\":\"IntelligentTieringRule\",\"Status\":\"Enabled\",\"Filter\":{},\"Transitions\":[{\"Days\":0,\"StorageClass\":\"INTELLIGENT_TIERING\"}]}]}'", bucketName),
			}

			f := models.Finding{
				ID:                fmt.Sprintf("s3-no-lifecycle-%s", bucketName),
				Category:          models.CategoryStorage,
				Severity:          models.DetermineSeverity(estimatedSavings),
				ResourceType:      "s3",
				ResourceID:        bucketName,
				ResourceName:      bucketName,
				Region:            "global",
				AccountID:         s.account,
				Tags:              map[string]string{"BucketName": bucketName},
				MonthlyWasteCost:  estimatedSavings,
				AnnualWasteCost:   estimatedSavings * 12.0,
				EstimatedSavings:  estimatedSavings,
				SavingsPercentage: 30.0,
				Recommendation:    rec,
				RemediationSteps:  remediation,
				Confidence:        "medium",
				DetectedAt:        now,
				Metrics: map[string]float64{
					"size_gb":           sizeGB,
					"monthly_cost":      monthlyCost,
					"estimated_savings": estimatedSavings,
				},
			}

			findings = append(findings, f)
		}
	}

	return findings, nil
}

func (s *S3Analyzer) getBucketSizeGB(ctx context.Context, bucketName string) float64 {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -3)

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/S3"),
		MetricName: aws.String("BucketSizeBytes"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("BucketName"), Value: aws.String(bucketName)},
			{Name: aws.String("StorageType"), Value: aws.String("StandardStorage")},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(now),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	out, err := s.cwClient.GetMetricStatistics(ctx, input)
	if err != nil || len(out.Datapoints) == 0 {
		return 50.0 // Default baseline size if CW metric is delayed
	}

	latestBytes := 0.0
	for _, dp := range out.Datapoints {
		if dp.Average != nil && *dp.Average > latestBytes {
			latestBytes = *dp.Average
		}
	}

	return latestBytes / (1024 * 1024 * 1024)
}
