terraform {
  backend "s3" {
    bucket         = "aws-finops-tfstate-dev-123456789012"
    key            = "dev/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "aws-finops-tfstate-locks"
    encrypt        = true
  }
}
