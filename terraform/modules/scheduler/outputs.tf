output "s3_bucket_name" {
  value       = aws_s3_bucket.reports.id
  description = "Name of the S3 bucket storing FinOps reports"
}

output "ecs_cluster_name" {
  value       = aws_ecs_cluster.main.name
  description = "Name of the ECS cluster created for the scheduled task"
}

output "eventbridge_rule_arn" {
  value       = aws_cloudwatch_event_rule.daily.arn
  description = "ARN of the daily EventBridge schedule rule"
}
