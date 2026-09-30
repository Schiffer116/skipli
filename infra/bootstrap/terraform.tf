terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.66"
    }
  }

  required_version = ">= 1.2"
  backend "s3" {
    bucket       = "skipli-797848269974-ap-southeast-7-an"
    key          = "infra/bootstrap/terraform.tfstate"
    region       = "ap-southeast-7"
    use_lockfile = true
  }
}
