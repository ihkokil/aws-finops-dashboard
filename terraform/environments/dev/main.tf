terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

module "iam" {
  source      = "../../modules/iam"
  environment = "dev"
  account_id  = var.account_id
  github_repo = var.github_repo
}

module "scheduler" {
  source             = "../../modules/scheduler"
  environment        = "dev"
  account_id         = var.account_id
  region             = var.region
  task_role_arn      = module.iam.role_arn
  collector_image    = var.collector_image
  subnet_ids         = var.subnet_ids
  security_group_ids = var.security_group_ids
  slack_webhook_url  = var.slack_webhook_url
}
