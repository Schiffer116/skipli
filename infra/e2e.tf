# Lets agents sign in to Development unattended: SES receives mail for a
# dev-only subdomain and keeps it briefly in a private bucket, where
# scripts/e2e-code reads the sign-in code. Production gets none of it.
variable "e2e_mail" {
  description = "Receive sign-in mail for the E2E test address. Development only."
  type        = bool
  default     = false
}

locals {
  e2e_count       = var.e2e_mail ? 1 : 0
  e2e_mail_domain = "e2e.${var.ui_domain}"
}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "e2e_mail" {
  count         = local.e2e_count
  region        = "us-east-1"
  bucket        = "skipli-e2e-mail-${data.aws_caller_identity.current.account_id}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "e2e_mail" {
  count                   = local.e2e_count
  region                  = "us-east-1"
  bucket                  = aws_s3_bucket.e2e_mail[0].id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "e2e_mail" {
  count  = local.e2e_count
  region = "us-east-1"
  bucket = aws_s3_bucket.e2e_mail[0].id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "e2e_mail" {
  count  = local.e2e_count
  region = "us-east-1"
  bucket = aws_s3_bucket.e2e_mail[0].id

  rule {
    id     = "expire"
    status = "Enabled"
    filter {}

    expiration {
      days = 1
    }
  }
}

data "aws_iam_policy_document" "e2e_mail" {
  count = local.e2e_count

  statement {
    principals {
      type        = "Service"
      identifiers = ["ses.amazonaws.com"]
    }

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.e2e_mail[0].arn}/*"]

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = ["arn:aws:ses:us-east-1:${data.aws_caller_identity.current.account_id}:receipt-rule-set/skipli-e2e:receipt-rule/store"]
    }
  }
}

resource "aws_s3_bucket_policy" "e2e_mail" {
  count  = local.e2e_count
  region = "us-east-1"
  bucket = aws_s3_bucket.e2e_mail[0].id
  policy = data.aws_iam_policy_document.e2e_mail[0].json
}

resource "aws_route53_record" "e2e_mail_mx" {
  count    = local.e2e_count
  provider = aws.dns
  zone_id  = data.aws_route53_zone.hosted_zone.zone_id
  name     = local.e2e_mail_domain
  type     = "MX"
  ttl      = 600
  records  = ["10 inbound-smtp.us-east-1.amazonaws.com"]
}

resource "aws_ses_receipt_rule_set" "e2e" {
  count         = local.e2e_count
  region        = "us-east-1"
  rule_set_name = "skipli-e2e"
}

resource "aws_ses_receipt_rule" "e2e_store" {
  count         = local.e2e_count
  region        = "us-east-1"
  name          = "store"
  rule_set_name = aws_ses_receipt_rule_set.e2e[0].rule_set_name
  recipients    = ["e2e@${local.e2e_mail_domain}"]
  enabled       = true
  scan_enabled  = true

  s3_action {
    position          = 1
    bucket_name       = aws_s3_bucket.e2e_mail[0].id
    object_key_prefix = "e2e/"
  }

  depends_on = [aws_s3_bucket_policy.e2e_mail]
}

resource "aws_ses_active_receipt_rule_set" "e2e" {
  count         = local.e2e_count
  region        = "us-east-1"
  rule_set_name = aws_ses_receipt_rule_set.e2e[0].rule_set_name
}

output "e2e_mail_address" {
  value = var.e2e_mail ? "e2e@${local.e2e_mail_domain}" : null
}

output "e2e_mail_bucket" {
  value = var.e2e_mail ? aws_s3_bucket.e2e_mail[0].id : null
}
