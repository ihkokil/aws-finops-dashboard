# AWS FinOps Cost Savings Calculation Methodology

This document details the mathematical formulas, pricing lookup models, and assumptions used by the AWS FinOps Collector to compute cost waste and potential savings across cloud resources.

---

## 1. Resource Waste Calculation Formulas

### 1.1 EC2 Idle & Rightsizing
- **Idle EC2 Instances:** Defined as `avg CPU < 5%` and `max CPU < 10%` over a 14-day lookback period.
  $$\text{Monthly Waste} = \text{Hourly On-Demand Price} \times 730 \text{ hours}$$
- **Underutilized EC2 Rightsizing:** Defined as `avg CPU < 20%` and `max CPU < 40%`.
  $$\text{Estimated Monthly Savings} = \left(\text{Current Type Rate} - \text{Recommended Type Rate}\right) \times 730$$

### 1.2 EBS Storage Waste
- **Unattached Volumes:** Volumes in `available` state for $>0$ days.
  $$\text{Monthly Waste} = \text{Volume Size (GB)} \times \text{Storage Class Rate/GB-Month}$$
  *Rates:* `gp3` = $0.08/GB, `gp2` = $0.10/GB, `io1` = $0.125/GB.
- **Orphaned Snapshots:** Snapshots $>90$ days old with no associated active AMI.
  $$\text{Monthly Waste} = \text{Snapshot Size (GB)} \times \$0.05/\text{GB-Month}$$

### 1.3 RDS Idle & Multi-AZ
- **Idle RDS Instances:** Zero database connections (`avg DatabaseConnections < 1.0`) over 14 days.
  $$\text{Monthly Waste} = \text{Instance Class Hourly Rate} \times 730 \times (2 \text{ if Multi-AZ})$$
- **Non-Prod Multi-AZ Premium:** Non-production instances (dev/staging/test) configured as Multi-AZ.
  $$\text{Monthly Savings} = \text{Total Monthly Cost} \times 50\%$$

### 1.4 S3 Lifecycle Policy Gaps
- **Buckets $>1\text{ GB}$ lacking lifecycle rules:**
  $$\text{Estimated Savings} = \text{Total Bucket Storage Cost} \times 30\%$$
  *(Reflects standard transition savings achieved via S3 Intelligent-Tiering).*

### 1.5 NAT Gateway & VPC Endpoints
- **Data Transfer Processing Fee:** NAT Gateways charge $\$0.045/\text{GB}$ data processing plus $\$0.045/\text{hour}$ baseline ($32.40/month).
- **VPC Endpoint Alternative:** Replacing S3/ECR internet access with VPC Gateway/Interface Endpoints eliminates per-GB processing fees.
  $$\text{Monthly Savings} = \text{NAT Data Processing Spend} \times 40\%$$

---

## 2. Realistic Savings Discount Factor (70%)

The tool calculates **Potential Realizable Savings** as **70% of Total Identified Waste**:

$$\text{Realizable Savings} = \text{Total Monthly Waste} \times 0.70$$

### Rationale for the 70% Multiplier:
1. **Business Constraints & Edge Cases:** Some low-CPU instances host critical stand-by or seasonal batch services that cannot be terminated.
2. **Testing & QA Overheads:** Rightsizing instance classes requires non-production validation to verify memory and I/O buffer margins.
3. **Contractual & Reserved Commitments:** Existing Savings Plans or RIs may reduce effective hourly rates below standard on-demand pricing.
