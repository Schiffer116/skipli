variable "environment" {
  type = string
}

variable "region" {
  type = string
}

variable "state_bucket" {
  type = string
}

variable "state_region" {
  type = string
}

variable "dns_role_arn" {
  type = string
}

variable "email_from_name" {
  type = string
}

variable "local_dev_principals" {
  type    = list(string)
  default = []
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      Project     = "Skipli"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

provider "aws" {
  alias  = "dns"
  region = "us-east-1"
  assume_role {
    role_arn = var.dns_role_arn
  }
}

data "aws_s3_bucket" "skipli" {
  bucket = var.state_bucket
  region = var.state_region
}

output "region" {
  value = var.region
}
