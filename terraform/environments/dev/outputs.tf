output "collector_role_arn" {
  value       = module.iam.role_arn
  description = "IAM Role ARN for FinOps Collector"
}

output "report_s3_bucket" {
  value       = module.scheduler.s3_bucket_name
  description = "S3 bucket for storing generated FinOps reports"
}
