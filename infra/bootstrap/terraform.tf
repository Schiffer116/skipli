terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.66"
    }

    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }

  required_version = ">= 1.2"
  backend "s3" {
    use_lockfile = true
  }
}
