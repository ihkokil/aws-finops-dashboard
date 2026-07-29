variable "environment" {
  type        = string
  description = "Target deployment environment (e.g., dev, prod)"
}

variable "account_id" {
  type        = string
  description = "AWS Account ID for role trusts"
}

variable "github_repo" {
  type        = string
  default     = ""
  description = "GitHub repository formatted as owner/repo for OIDC trust condition"
}
