variable "account_id" {
  type        = string
  description = "AWS Account ID"
}

variable "region" {
  type        = string
  default     = "us-east-1"
  description = "AWS Region"
}

variable "github_repo" {
  type        = string
  default     = "owner/aws-finops-dashboard"
  description = "GitHub Repository string"
}

variable "collector_image" {
  type        = string
  default     = "123456789012.dkr.ecr.us-east-1.amazonaws.com/finops-collector:latest"
  description = "Collector container image"
}

variable "subnet_ids" {
  type    = list(string)
  default = ["subnet-0123456789prod1", "subnet-0123456789prod2"]
}

variable "security_group_ids" {
  type    = list(string)
  default = ["sg-0123456789prod0"]
}

variable "slack_webhook_url" {
  type      = string
  default   = ""
  sensitive = true
}
