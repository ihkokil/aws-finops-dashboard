# AWS FinOps Findings & Waste Taxonomy Guide

This guide explains the classification schema, severity thresholds, and resource checks performed by the AWS FinOps collector.

---

## 1. Severity Threshold Matrix

| Severity Level | Monthly Waste Threshold | Action Required |
|----------------|--------------------------|-----------------|
| **🔴 Critical** | $\ge \$100/\text{month}$ | Immediate action within 48 hours. Triggers automated GitHub Issue creation. |
| **🟠 High** | $\$50 - \$99.99/\text{month}$ | Action required within current sprint (14 days). |
| **🟡 Medium** | $\$10 - \$49.99/\text{month}$ | Review during monthly FinOps cadence. |
| **🔵 Low** | $<\$10/\text{month}$ | Informational finding; remediate during batch cleanup. |

---

## 2. Resource Checks & Category Mapping

### 2.1 Category: `idle`
- **EC2 Idle Instances:** Running EC2 instance with `avg CPU < 5%` AND `max CPU < 10%` over 14 days.
- **RDS Idle Databases:** Database instance with `avg DatabaseConnections < 1.0` over 14 days.
- **ELB Idle Load Balancers:** Application Load Balancer with $<100$ requests/day or Network Load Balancer with $<1\text{ MB}$ processed data/day.

### 2.2 Category: `rightsizing`
- **EC2 Underutilized Instances:** Running EC2 instance with `avg CPU < 20%` AND `max CPU < 40%`.
- **Cost Explorer Rightsizing Recommendations:** Recommendations generated directly by AWS Cost Explorer Machine Learning models.
- **Non-Prod Multi-AZ RDS:** Databases tagged `dev`/`test`/`sandbox` running in multi-availability-zone redundancy mode.

### 2.3 Category: `storage`
- **EBS Unattached Volumes:** Volume state is `available` (unattached to any EC2 instance).
- **Orphaned Snapshots:** Snapshot $>90$ days old with no matching active AMI.
- **S3 Missing Lifecycle Rules:** Buckets $>1\text{ GB}$ lacking lifecycle transitions to Intelligent-Tiering or Glacier.

### 2.4 Category: `network`
- **NAT Gateway High Processing Spend:** NAT Gateway with spend $>\$50/\text{month}$ lacking VPC Endpoints for high-volume S3/ECR traffic.
- **Non-Prod 24/7 NAT Gateways:** NAT Gateways running in non-prod VPCs without scheduled off-hour teardown.
