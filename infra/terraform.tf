terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }

    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.66"
    }

    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.8"
    }

    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }

  required_version = ">= 1.2"
  backend "s3" {
    bucket       = "skipli-797848269974-ap-southeast-7-an"
    key          = "infra/terraform.tfstate"
    region       = "ap-southeast-7"
    use_lockfile = true
  }
}
