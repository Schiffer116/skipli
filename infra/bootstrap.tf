provider "github" {
  token = var.github_token
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      Project   = "Skipli"
      ManagedBy = "terraform"
    }
  }
}

variable "region" {
  type    = string
  default = "ap-southeast-7"
}

variable "github_token" {
  description = "GitHub token for authentication"
  type        = string
  sensitive   = true
}

locals {
  github_repository_name = "skipli"
  state_region           = "ap-southeast-7"
}

output "region" {
  value = var.region
}

resource "github_actions_variable" "github_actions_role_arn" {
  repository    = local.github_repository_name
  variable_name = "AWS_GITHUB_ACTIONS_ROLE_ARN"
  value         = aws_iam_role.github_actions.arn
}

resource "github_actions_variable" "aws_region" {
  repository    = local.github_repository_name
  variable_name = "AWS_REGION"
  value         = local.state_region
}

resource "github_actions_variable" "skipli_bucket_name" {
  repository    = local.github_repository_name
  variable_name = "SKIPLI_BUCKET_NAME"
  value         = aws_s3_bucket.skipli.bucket
}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "skipli" {
  region           = local.state_region
  bucket           = "skipli-${data.aws_caller_identity.current.account_id}-${local.state_region}-an"
  bucket_namespace = "account-regional"
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "skipli" {
  region = local.state_region
  bucket = aws_s3_bucket.skipli.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_iam_openid_connect_provider" "github_oidc" {
  url = "https://token.actions.githubusercontent.com"

  client_id_list = [
    "sts.amazonaws.com",
  ]
}

data "aws_iam_policy_document" "github_trust_policy" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github_oidc.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:Schiffer116/skipli:ref:refs/heads/master"]
    }
  }

  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "AWS"
      identifiers = [data.aws_caller_identity.current.account_id]
    }
  }
}

data "aws_iam_policy_document" "allow_skipli_s3" {
  statement {
    effect    = "Allow"
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.skipli.arn]
  }

  statement {
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
    ]
    resources = ["${aws_s3_bucket.skipli.arn}/*"]
  }
}

resource "aws_iam_policy" "allow_skipli_s3" {
  policy = data.aws_iam_policy_document.allow_skipli_s3.json
}

resource "aws_iam_role" "github_actions" {
  name               = "GithubDeployRole"
  assume_role_policy = data.aws_iam_policy_document.github_trust_policy.json
}

resource "aws_iam_role_policy_attachment" "attach_policy" {
  role       = aws_iam_role.github_actions.name
  policy_arn = aws_iam_policy.allow_skipli_s3.id
}
