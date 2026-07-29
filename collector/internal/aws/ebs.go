package aws

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws-finops-dashboard/collector/internal/models"
)

var ebsGBMonthlyRates = map[string]float64{
	"gp3":    0.08,
	"gp2":    0.10,
	"io1":    0.125,
	"io2":    0.125,
	"st1":    0.045,
	"sc1":    0.015,
	"standard": 0.05,
}

// EBSAnalyzer identifies unattached storage volumes and orphaned snapshots.
type EBSAnalyzer struct {
	ec2Client *ec2.Client
	region    string
	accountID string
}

// NewEBSAnalyzer creates an EBS analyzer instance.
func NewEBSAnalyzer(cfg aws.Config, region, accountID string) *EBSAnalyzer {
	return &EBSAnalyzer{
		ec2Client: ec2.NewFromConfig(cfg),
		region:    region,
		accountID: accountID,
	}
}

// FindUnattachedVolumes discovers EBS volumes in "available" state.
func (e *EBSAnalyzer) FindUnattachedVolumes(ctx context.Context) ([]models.Finding, error) {
	input := &ec2.DescribeVolumesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("status"),
				Values: []string{"available"},
			},
		},
	}

	out, err := e.ec2Client.DescribeVolumes(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe EBS volumes: %w", err)
	}

	var findings []models.Finding
	now := time.Now().UTC()

	for _, vol := range out.Volumes {
		volumeID := aws.ToString(vol.VolumeId)
		sizeGB := float64(aws.ToInt32(vol.Size))
		volType := string(vol.VolumeType)
		az := aws.ToString(vol.AvailabilityZone)

		createTime := time.Now()
		if vol.CreateTime != nil {
			createTime = *vol.CreateTime
		}
		daysUnattached := float64(int(now.Sub(createTime).Hours() / 24))

		rate, ok := ebsGBMonthlyRates[volType]
		if !ok {
			rate = 0.08
		}
		monthlyCost := sizeGB * rate

		tags := make(map[string]string)
		volName := volumeID
		for _, tag := range vol.Tags {
			k := aws.ToString(tag.Key)
			v := aws.ToString(tag.Value)
			tags[k] = v
			if k == "Name" && v != "" {
				volName = v
			}
		}

		severity := models.DetermineSeverity(monthlyCost)
		rec := fmt.Sprintf("Unattached EBS volume %s (%s, %.0f GB, %s) unattached for %.0f days. Delete volume or archive.", volName, volType, sizeGB, az, daysUnattached)
		remediation := []string{
			fmt.Sprintf("aws ec2 create-snapshot --volume-id %s --description 'Backup prior to deletion' --region %s", volumeID, e.region),
			fmt.Sprintf("aws ec2 delete-volume --volume-id %s --region %s", volumeID, e.region),
		}

		f := models.Finding{
			ID:                fmt.Sprintf("ebs-unattached-%s", volumeID),
			Category:          models.CategoryStorage,
			Severity:          severity,
			ResourceType:      "ebs",
			ResourceID:        volumeID,
			ResourceName:      volName,
			Region:            e.region,
			AccountID:         e.accountID,
			Tags:              tags,
			MonthlyWasteCost:  monthlyCost,
			AnnualWasteCost:   monthlyCost * 12.0,
			EstimatedSavings:  monthlyCost,
			SavingsPercentage: 100.0,
			Recommendation:    rec,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"size_gb":         sizeGB,
				"days_unattached": daysUnattached,
				"monthly_cost":    monthlyCost,
			},
		}

		findings = append(findings, f)
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].MonthlyWasteCost > findings[j].MonthlyWasteCost
	})

	return findings, nil
}

// FindOldSnapshots identifies EBS snapshots older than 90 days with no active AMI.
func (e *EBSAnalyzer) FindOldSnapshots(ctx context.Context) ([]models.Finding, error) {
	input := &ec2.DescribeSnapshotsInput{
		OwnerIds: []string{"self"},
	}

	out, err := e.ec2Client.DescribeSnapshots(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe snapshots: %w", err)
	}

	// Fetch active AMIs owned by self to check associated image IDs
	amiInput := &ec2.DescribeImagesInput{
		Owners: []string{"self"},
	}
	amiOut, _ := e.ec2Client.DescribeImages(ctx, amiInput)
	activeAMISnapshots := make(map[string]bool)
	if amiOut != nil {
		for _, img := range amiOut.Images {
			for _, bdm := range img.BlockDeviceMappings {
				if bdm.Ebs != nil && bdm.Ebs.SnapshotId != nil {
					activeAMISnapshots[*bdm.Ebs.SnapshotId] = true
				}
			}
		}
	}

	var findings []models.Finding
	now := time.Now().UTC()

	for _, snap := range out.Snapshots {
		snapID := aws.ToString(snap.SnapshotId)
		if activeAMISnapshots[snapID] {
			continue // Skip snapshots linked to an existing active AMI
		}

		startTime := time.Now()
		if snap.StartTime != nil {
			startTime = *snap.StartTime
		}
		daysOld := float64(int(now.Sub(startTime).Hours() / 24))

		if daysOld < 90 {
			continue // Only flag snapshots older than 90 days
		}

		sizeGB := float64(aws.ToInt32(snap.VolumeSize))
		monthlyCost := sizeGB * 0.05 // Standard EBS snapshot pricing ($0.05/GB-month)

		tags := make(map[string]string)
		snapName := snapID
		for _, t := range snap.Tags {
			k := aws.ToString(t.Key)
			v := aws.ToString(t.Value)
			tags[k] = v
			if k == "Name" && v != "" {
				snapName = v
			}
		}

		rec := fmt.Sprintf("Delete snapshot %s - no associated AMI, %.0f days old (%.0f GB)", snapName, daysOld, sizeGB)
		remediation := []string{
			fmt.Sprintf("aws ec2 delete-snapshot --snapshot-id %s --region %s", snapID, e.region),
		}

		f := models.Finding{
			ID:                fmt.Sprintf("ebs-snapshot-%s", snapID),
			Category:          models.CategoryStorage,
			Severity:          models.DetermineSeverity(monthlyCost),
			ResourceType:      "ebs",
			ResourceID:        snapID,
			ResourceName:      snapName,
			Region:            e.region,
			AccountID:         e.accountID,
			Tags:              tags,
			MonthlyWasteCost:  monthlyCost,
			AnnualWasteCost:   monthlyCost * 12.0,
			EstimatedSavings:  monthlyCost,
			SavingsPercentage: 100.0,
			Recommendation:    rec,
			RemediationSteps:  remediation,
			Confidence:        "high",
			DetectedAt:        now,
			Metrics: map[string]float64{
				"size_gb":      sizeGB,
				"days_old":     daysOld,
				"monthly_cost": monthlyCost,
			},
		}

		findings = append(findings, f)
	}

	return findings, nil
}
