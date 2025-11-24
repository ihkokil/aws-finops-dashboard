output "role_arn" {
  value       = aws_iam_role.collector.arn
  description = "ARN of the read-only FinOps collector IAM role"
}

output "role_name" {
  value       = aws_iam_role.collector.name
  description = "Name of the read-only FinOps collector IAM role"
}

output "policy_arn" {
  value       = aws_iam_policy.collector_policy.arn
  description = "ARN of the attached read-only policy"
}
