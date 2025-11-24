# AWS FinOps Remediation Playbook

Step-by-step AWS CLI instructions for remediating each category of cost waste identified by the FinOps collector.

---

## 1. EC2 Idle & Rightsizing Remediation

### 1.1 Stop or Terminate Idle EC2 Instances
```bash
# 1. Stop instance (preserves root disk data, stops compute billing)
aws ec2 stop-instances --instance-ids i-1234567890abcdef0 --region us-east-1

# 2. Terminate instance after confirming no active dependencies
aws ec2 terminate-instances --instance-ids i-1234567890abcdef0 --region us-east-1
```

### 1.2 Downsize Underutilized EC2 Instance
```bash
# Stop instance prior to modifying attribute
aws ec2 stop-instances --instance-ids i-1234567890abcdef0

# Modify instance type to recommended smaller size (e.g. m5.xlarge -> t3.medium)
aws ec2 modify-instance-attribute \
  --instance-id i-1234567890abcdef0 \
  --instance-type t3.medium

# Start instance
aws ec2 start-instances --instance-ids i-1234567890abcdef0
```

---

## 2. EBS Volume & Snapshot Remediation

### 2.1 Backup & Delete Unattached EBS Volume
```bash
# 1. Create safety snapshot first
aws ec2 create-snapshot \
  --volume-id vol-0123456789abcdef0 \
  --description "pre-deletion-backup" \
  --region us-east-1

# 2. Delete unattached volume
aws ec2 delete-volume \
  --volume-id vol-0123456789abcdef0 \
  --region us-east-1
```

### 2.2 Delete Orphaned EBS Snapshot
```bash
aws ec2 delete-snapshot \
  --snapshot-id snap-0123456789abcdef0 \
  --region us-east-1
```

---

## 3. RDS Database Remediation

### 3.1 Stop Idle RDS Database Instance
```bash
aws rds stop-db-instance \
  --db-instance-identifier prod-reporting-db \
  --region us-east-1
```

### 3.2 Disable Multi-AZ on Non-Production Instance
```bash
aws rds modify-db-instance \
  --db-instance-identifier dev-test-db \
  --no-multi-az \
  --apply-immediately
```

---

## 4. S3 Storage Lifecycle Remediation

### 4.1 Apply Intelligent-Tiering Lifecycle Policy
```bash
aws s3api put-bucket-lifecycle-configuration \
  --bucket my-company-data-bucket \
  --lifecycle-configuration '{
    "Rules": [
      {
        "ID": "AutoIntelligentTiering",
        "Status": "Enabled",
        "Filter": {},
        "Transitions": [
          {
            "Days": 0,
            "StorageClass": "INTELLIGENT_TIERING"
          }
        ]
      }
    ]
  }'
```

---

## 5. NAT Gateway Data Cost Remediation

### 5.1 Create Gateway VPC Endpoints for S3 & DynamoDB
```bash
aws ec2 create-vpc-endpoint \
  --vpc-id vpc-0123456789abcdef0 \
  --service-name com.amazonaws.us-east-1.s3 \
  --vpc-endpoint-type Gateway \
  --region us-east-1
```
