variable "environment" {
  type        = string
  description = "Target deployment environment"
}

variable "account_id" {
  type        = string
  description = "AWS Account ID"
}

variable "region" {
  type        = string
  description = "AWS Region"
}

variable "task_role_arn" {
  type        = string
  description = "IAM Role ARN for ECS Task Execution"
}

variable "collector_image" {
  type        = string
  description = "ECR Image URI for collector container"
}

variable "subnet_ids" {
  type        = list(string)
  description = "VPC Subnet IDs for ECS Fargate task execution"
}

variable "security_group_ids" {
  type        = list(string)
  description = "Security Group IDs for ECS Fargate task execution"
}

variable "slack_webhook_url" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Slack Webhook URL for posting report summaries"
}
