terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# Trust policy for GitHub Actions OIDC & ECS Task Execution
data "aws_iam_policy_document" "trust" {
  # ECS Task Trust
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }

  # GitHub Actions OIDC Trust
  dynamic "statement" {
    for_each = var.github_repo != "" ? [1] : []
    content {
      effect  = "Allow"
      actions = ["sts:AssumeRoleWithWebIdentity"]

      principals {
        type        = "Federated"
        identifiers = ["arn:aws:iam::${var.account_id}:oidc-provider/token.actions.githubusercontent.com"]
      }

      condition {
        test     = "StringEquals"
        variable = "token.actions.githubusercontent.com:aud"
        values   = ["sts.amazonaws.com"]
      }

      condition {
        test     = "StringLike"
        variable = "token.actions.githubusercontent.com:sub"
        values   = ["repo:${var.github_repo}:*"]
      }
    }
  }
}

resource "aws_iam_role" "collector" {
  name               = "${var.environment}-finops-collector-role"
  assume_role_policy = data.aws_iam_policy_document.trust.json

  tags = {
    Environment = var.environment
    ManagedBy   = "Terraform"
    Project     = "aws-finops-dashboard"
  }
}

# Read-Only Policy Document
data "aws_iam_policy_document" "read_only" {
  statement {
    sid    = "CostExplorerReadOnly"
    effect = "Allow"
    actions = [
      "ce:GetCostAndUsage",
      "ce:GetCostForecast",
      "ce:GetRightsizingRecommendation",
      "ce:GetSavingsPlanUtilization",
      "ce:GetReservationUtilization"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "EC2ReadOnly"
    effect = "Allow"
    actions = [
      "ec2:DescribeInstances",
      "ec2:DescribeVolumes",
      "ec2:DescribeSnapshots",
      "ec2:DescribeNatGateways",
      "ec2:DescribeVpcEndpoints"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "RDSReadOnly"
    effect = "Allow"
    actions = [
      "rds:DescribeDBInstances",
      "rds:DescribeDBClusters"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "ELBReadOnly"
    effect = "Allow"
    actions = [
      "elasticloadbalancing:DescribeLoadBalancers",
      "elasticloadbalancing:DescribeTargetGroups",
      "elasticloadbalancing:DescribeTargetHealth"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "S3ReadOnly"
    effect = "Allow"
    actions = [
      "s3:ListAllMyBuckets",
      "s3:GetBucketLocation",
      "s3:GetBucketLifecycleConfiguration",
      "s3:GetBucketTagging"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "CloudWatchReadOnly"
    effect = "Allow"
    actions = [
      "cloudwatch:GetMetricStatistics",
      "cloudwatch:GetMetricData",
      "cloudwatch:ListMetrics"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "STSIdentity"
    effect = "Allow"
    actions = [
      "sts:GetCallerIdentity"
    ]
    resources = ["*"]
  }

  # Explicit DENY on all mutation / destructive actions
  statement {
    sid    = "ExplicitDenyDestructiveActions"
    effect = "Deny"
    actions = [
      "ec2:TerminateInstances",
      "ec2:DeleteVolume",
      "ec2:DeleteSnapshot",
      "rds:DeleteDBInstance",
      "s3:DeleteObject",
      "s3:DeleteBucket"
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "collector_policy" {
  name        = "${var.environment}-finops-collector-policy"
  description = "Strict read-only IAM policy for AWS FinOps collector with explicit deny rules"
  policy      = data.aws_iam_policy_document.read_only.json
}

resource "aws_iam_role_policy_attachment" "attach" {
  role       = aws_iam_role.collector.name
  policy_arn = aws_iam_policy.collector_policy.arn
}
