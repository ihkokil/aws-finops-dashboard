terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# S3 Report Bucket
resource "aws_s3_bucket" "reports" {
  bucket        = "finops-reports-${var.account_id}-${var.environment}"
  force_destroy = var.environment == "dev" ? true : false

  tags = {
    Environment = var.environment
    Project     = "aws-finops-dashboard"
  }
}

resource "aws_s3_bucket_versioning" "reports" {
  bucket = aws_s3_bucket.reports.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "reports" {
  bucket = aws_s3_bucket.reports.id

  rule {
    id     = "expire-reports-365-days"
    status = "Enabled"
    filter {}

    expiration {
      days = 365
    }
  }
}

# CloudWatch Log Group for ECS Task
resource "aws_cloudwatch_log_group" "ecs" {
  name              = "/ecs/${var.environment}-finops-collector"
  retention_in_days = 30
}

# ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = "${var.environment}-finops-cluster"
}

# ECS Task Definition
resource "aws_ecs_task_definition" "collector" {
  family                   = "${var.environment}-finops-collector"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = var.task_role_arn
  task_role_arn            = var.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "collector"
      image     = var.collector_image
      essential = true
      command   = ["--output-format", "all", "--output-dir", "/reports"]
      environment = [
        { name = "AWS_REGION", value = var.region },
        { name = "OUTPUT_DIR", value = "/reports" }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.ecs.name
          "awslogs-region"        = var.region
          "awslogs-stream-prefix" = "collector"
        }
      }
    }
  ])
}

# EventBridge Scheduler Rule
resource "aws_cloudwatch_event_rule" "daily" {
  name                = "${var.environment}-finops-daily-rule"
  description         = "Trigger FinOps Collector daily at 06:00 UTC"
  schedule_expression = "cron(0 6 * * ? *)"
}

# EventBridge Target to ECS Task
resource "aws_cloudwatch_event_target" "ecs" {
  rule     = aws_cloudwatch_event_rule.daily.name
  arn      = aws_ecs_cluster.main.arn
  role_arn = var.task_role_arn

  ecs_target {
    task_definition_arn = aws_ecs_task_definition.collector.arn
    task_count          = 1
    launch_type         = "FARGATE"

    network_configuration {
      subnets          = var.subnet_ids
      security_groups  = var.security_group_ids
      assign_public_ip = true
    }
  }
}

# Lambda Execution Role for Slack Notifications
resource "aws_iam_role" "lambda_role" {
  name = "${var.environment}-finops-slack-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_basic" {
  role       = aws_iam_role.lambda_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "lambda_s3_read" {
  name = "s3-read-reports"
  role = aws_iam_role.lambda_role.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = "${aws_s3_bucket.reports.arn}/*"
    }]
  })
}

# Inline Python Lambda for Slack Post
data "archive_file" "lambda_zip" {
  type        = "zip"
  output_path = "${path.module}/slack_notifier.zip"

  source {
    content  = <<EOF
import json
import urllib.request
import os
import boto3

s3 = boto3.client('s3')

def handler(event, context):
    slack_url = os.environ.get('SLACK_WEBHOOK_URL')
    if not slack_url:
        return {'statusCode': 200, 'body': 'No Slack URL'}
    
    bucket = event['Records'][0]['s3']['bucket']['name']
    key = event['Records'][0]['s3']['object']['key']
    
    if not key.endswith('.md'):
        return {'statusCode': 200, 'body': 'Ignored non-markdown file'}
        
    response = s3.get_object(Bucket=bucket, Key=key)
    content = response['Body'].read().decode('utf-8')
    
    summary_text = f"📊 *New AWS FinOps Daily Report Generated*\nBucket: `{bucket}`\n\n```\n" + content[:500] + "\n```"
    
    payload = json.dumps({"text": summary_text}).encode('utf-8')
    req = urllib.request.Request(slack_url, data=payload, headers={'Content-Type': 'application/json'})
    urllib.request.urlopen(req)
    
    return {'statusCode': 200, 'body': 'Slack notified'}
EOF
    filename = "index.py"
  }
}

resource "aws_lambda_function" "slack_notifier" {
  filename         = data.archive_file.lambda_zip.output_path
  function_name    = "${var.environment}-finops-slack-notifier"
  role             = aws_iam_role.lambda_role.arn
  handler          = "index.handler"
  runtime          = "python3.11"
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256

  environment {
    variables = {
      SLACK_WEBHOOK_URL = var.slack_webhook_url
    }
  }
}

# S3 Notification to Lambda
resource "aws_lambda_permission" "allow_s3" {
  statement_id  = "AllowExecutionFromS3Bucket"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.slack_notifier.arn
  principal     = "s3.amazonaws.com"
  source_arn    = aws_s3_bucket.reports.arn
}

resource "aws_s3_bucket_notification" "bucket_notification" {
  bucket = aws_s3_bucket.reports.id

  lambda_function {
    lambda_function_arn = aws_lambda_function.slack_notifier.arn
    events              = ["s3:ObjectCreated:*"]
    filter_suffix       = ".md"
  }

  depends_on = [aws_lambda_permission.allow_s3]
}
