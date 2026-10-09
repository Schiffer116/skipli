variable "environment" {
  type = string
}

variable "state_region" {
  type = string
}

variable "github_token" {
  type      = string
  sensitive = true
}

variable "github_owner" {
  type    = string
  default = "Schiffer116"
}

variable "github_repository" {
  type    = string
  default = "skipli"
}

variable "hosted_zone" {
  type    = string
  default = "schifferarchitecture.com"
}

variable "production_account_id" {
  type    = string
  default = ""
}

provider "aws" {
  region = var.state_region
  default_tags {
    tags = {
      Project     = "Skipli"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

provider "github" {
  token = var.github_token
  owner = var.github_owner
}

locals {
  is_dev             = var.environment == "dev"
  github_environment = local.is_dev ? "development" : "production"
  deploy_branch      = local.is_dev ? "dev" : "master"
  variable_prefix    = upper(var.environment)
  repo               = "${var.github_owner}/${var.github_repository}"
  oidc_subjects = local.is_dev ? [
    "repo:${local.repo}:environment:development",
    ] : [
    "repo:${local.repo}:ref:refs/heads/master",
    "repo:${local.repo}:environment:production",
  ]
}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "state" {
  bucket           = "skipli-${data.aws_caller_identity.current.account_id}-${var.state_region}-an"
  bucket_namespace = "account-regional"
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_iam_openid_connect_provider" "github" {
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
}

data "aws_iam_policy_document" "deploy_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = local.oidc_subjects
    }
  }
}

resource "aws_iam_role" "deploy" {
  name               = "GithubDeployRole"
  assume_role_policy = data.aws_iam_policy_document.deploy_trust.json
}

resource "aws_iam_role_policy_attachment" "deploy_admin" {
  role       = aws_iam_role.deploy.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

data "github_user" "reviewer" {
  username = var.github_owner
}

resource "github_repository_environment" "deploy" {
  repository          = var.github_repository
  environment         = local.github_environment
  prevent_self_review = false

  dynamic "reviewers" {
    for_each = local.is_dev ? [] : [1]
    content {
      users = [data.github_user.reviewer.id]
    }
  }

  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "deploy" {
  repository     = var.github_repository
  environment    = github_repository_environment.deploy.environment
  branch_pattern = local.deploy_branch
}

resource "github_actions_variable" "deploy_role_arn" {
  repository    = var.github_repository
  variable_name = "${local.variable_prefix}_DEPLOY_ROLE_ARN"
  value         = aws_iam_role.deploy.arn
}

resource "github_actions_variable" "aws_region" {
  repository    = var.github_repository
  variable_name = "${local.variable_prefix}_AWS_REGION"
  value         = var.state_region
}

resource "github_actions_variable" "ui_bucket" {
  repository    = var.github_repository
  variable_name = "${local.variable_prefix}_UI_BUCKET"
  value         = aws_s3_bucket.state.bucket
}

data "aws_route53_zone" "zone" {
  count = local.is_dev ? 1 : 0
  name  = var.hosted_zone
}

data "aws_iam_policy_document" "dns_trust" {
  count = local.is_dev ? 1 : 0
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "AWS"
      identifiers = [data.aws_caller_identity.current.account_id, var.production_account_id]
    }
  }
}

data "aws_iam_policy_document" "dns" {
  count = local.is_dev ? 1 : 0
  statement {
    actions = [
      "route53:ChangeResourceRecordSets",
      "route53:ListResourceRecordSets",
      "route53:GetHostedZone",
      "route53:ListTagsForResource",
    ]
    resources = [data.aws_route53_zone.zone[0].arn]
  }
  statement {
    actions   = ["route53:GetChange"]
    resources = ["arn:aws:route53:::change/*"]
  }
  statement {
    actions   = ["route53:ListHostedZones", "route53:ListHostedZonesByName"]
    resources = ["*"]
  }
}

resource "aws_iam_role" "dns" {
  count              = local.is_dev ? 1 : 0
  name               = "SkipliDns"
  assume_role_policy = data.aws_iam_policy_document.dns_trust[0].json
}

resource "aws_iam_role_policy" "dns" {
  count  = local.is_dev ? 1 : 0
  name   = "SkipliDnsRecords"
  role   = aws_iam_role.dns[0].id
  policy = data.aws_iam_policy_document.dns[0].json
}

resource "aws_route53_record" "dmarc" {
  count   = local.is_dev ? 1 : 0
  zone_id = data.aws_route53_zone.zone[0].zone_id
  name    = "_dmarc.${var.hosted_zone}"
  type    = "TXT"
  ttl     = 600
  records = ["v=DMARC1; p=none;"]
}

resource "github_repository_ruleset" "master" {
  count       = local.is_dev ? 1 : 0
  name        = "master"
  repository  = var.github_repository
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/master"]
      exclude = []
    }
  }

  rules {
    deletion         = true
    non_fast_forward = true

    pull_request {
      required_approving_review_count = 0
    }

    required_status_checks {
      required_check {
        context = "ui"
      }
      required_check {
        context = "api"
      }
      required_check {
        context = "infra"
      }
      required_check {
        context = "pr-source"
      }
    }
  }
}

output "deploy_role_arn" {
  value = aws_iam_role.deploy.arn
}

output "state_bucket" {
  value = aws_s3_bucket.state.bucket
}

output "dns_role_arn" {
  value = local.is_dev ? aws_iam_role.dns[0].arn : null
}
