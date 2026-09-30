provider "aws" {
  region = "ap-southeast-7"
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

resource "aws_s3_bucket" "skipli" {
  bucket           = "skipli-${data.aws_caller_identity.current.account_id}-${data.aws_region.current.region}-an"
  bucket_namespace = "account-regional"
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "skipli" {
  bucket = aws_s3_bucket.skipli.id
  versioning_configuration {
    status = "Enabled"
  }
}

# For Github actions
resource "aws_ssm_parameter" "bucket_name" {
  name  = "/skipli/bucket-name"
  type  = "String"
  value = aws_s3_bucket.skipli.bucket
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
    resources = ["${aws_s3_bucket.skipli.arn}/ui/*"]
  }

  statement {
    effect    = "Allow"
    actions   = ["ssm:GetParameter"]
    resources = [aws_ssm_parameter.bucket_name.arn]
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

output "github_actions_role_arn" {
  description = "IAM role for Github actions"
  value       = aws_iam_role.github_actions.arn
}

output "skipli_bucket" {
  description = "Skipli S3 bucket ARN"
  value       = aws_s3_bucket.skipli.arn
}
