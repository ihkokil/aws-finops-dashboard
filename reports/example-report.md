# AWS FinOps Report — 2026-07-29
**Account:** `123456789012` | **Region:** `us-east-1` | **Period:** 14 Days

## Executive Summary

| Metric | Value |
|--------|-------|
| Total Monthly Spend | $6,247.50 |
| Identified Waste | $1,284.30 (20.6%) |
| Annual Waste Projection | $15,411.60 |
| Realistic Potential Savings (70%) | $899.01/month |
| Critical Findings (>$100/mo) | 3 |
| High Severity Findings (>$50/mo) | 7 |
| Medium Severity Findings (>$10/mo) | 12 |

## 🔴 Critical Findings (>$100/month waste)

### 1. Idle RDS Cluster — prod-reporting-db
- **Monthly Waste:** $445.00
- **Resource ID:** `prod-reporting-db` (rds)
- **Recommendation:** Idle RDS instance prod-reporting-db (db.r5.xlarge) — avg 0.0 database connections over 14 days. Stop instance outside business hours or migrate to Aurora Serverless v2.
- **Remediation:**
```bash
aws rds stop-db-instance --db-instance-identifier prod-reporting-db --region us-east-1
```

### 2. Idle EC2 Instance — analytics-batch-worker
- **Monthly Waste:** $279.60
- **Resource ID:** `i-0a8b9c1d2e3f4a5b6` (ec2)
- **Recommendation:** Idle EC2 instance analytics-batch-worker (m5.2xlarge) — avg CPU 1.2%, max CPU 3.8% over 14 days. Stop or terminate instance.
- **Remediation:**
```bash
aws ec2 stop-instances --instance-ids i-0a8b9c1d2e3f4a5b6 --region us-east-1
```

### 3. NAT Gateway Processing Fees — dev-vpc-nat
- **Monthly Waste:** $186.40
- **Resource ID:** `nat-0123456789abcdef0` (nat)
- **Recommendation:** NAT Gateway dev-vpc-nat ($240.50/month, 3,420 GB processed) lacks S3 Gateway Endpoint in VPC vpc-0a1b2c3d4e. Create free Gateway Endpoint to eliminate data processing fees.
- **Remediation:**
```bash
aws ec2 create-vpc-endpoint --vpc-id vpc-0a1b2c3d4e --service-name com.amazonaws.us-east-1.s3 --vpc-endpoint-type Gateway --region us-east-1
```

## 🟠 High Severity Findings (>$50/month waste)

### 1. Unattached EBS Volume — vol-backup-disk-old (ebs)
- **Monthly Waste:** $80.00
- **Recommendation:** Unattached EBS volume vol-backup-disk-old (gp2, 800 GB, us-east-1a) unattached for 45 days. Delete volume after snapshot.

### 2. Non-Prod Multi-AZ RDS — staging-user-db (rds)
- **Monthly Waste:** $76.80
- **Recommendation:** Non-prod Multi-AZ RDS instance staging-user-db (db.m5.large) — disable Multi-AZ for 50% cost reduction ($76.80/month savings).

## 🟡 Medium Severity Findings (>$10/month waste)

- **S3 Bucket Without Lifecycle Policy** (`logs-archive-raw-data`): $34.50/month — Add S3 Intelligent-Tiering rule.
- **Idle Load Balancer** (`alb-staging-legacy`): $16.20/month — 0 requests in 14 days. Delete load balancer.
- **Orphaned EBS Snapshot** (`snap-0fe987654321`): $12.50/month — Delete snapshot (no associated AMI, 120 days old).

## Savings Plan & Reserved Instance Utilization

- **Savings Plan Utilization:** 73.0% ⚠️ (Below 80% target threshold)
- **Unutilized Commitment Waste:** $162.00/month
- **Reserved Instance Coverage:** 81.5%
- **RI Unused Hours:** 142 hours

## Top 10 Services by Spend

| Service | Monthly Cost | Usage Quantity |
|---------|--------------|----------------|
| Amazon Elastic Compute Cloud - Compute | $2,410.50 | 18,420 Hours |
| Amazon Relational Database Service | $1,650.00 | 2,880 Hours |
| Amazon Simple Storage Service | $890.20 | 38,700 GB-Month |
| Amazon EC2-Other (EBS & NAT) | $620.40 | 14,200 GB |
| Amazon ElastiCache | $240.00 | 1,440 Hours |
| AWS OpenSearch Service | $185.00 | 720 Hours |
| Amazon Route 53 | $65.00 | 120 Hosted Zones |
| AWS CloudTrail | $48.00 | 1,200 GB |
| Amazon Key Management Service | $38.40 | 3,840 Keys |
| AWS CloudWatch | $100.00 | 500 Metrics |

## 30-Day Forecast

- **Predicted Spend:** $6,410.00
- **Confidence Interval:** $6,120.00 – $6,780.00
