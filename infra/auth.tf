# Cognito and SES live in us-east-1: SES isn't available in ap-southeast-7,
# and Cognito's email codes are sent through SES.

resource "aws_sesv2_email_identity" "domain" {
  region         = "us-east-1"
  email_identity = data.aws_route53_zone.hosted_zone.name
}

resource "aws_route53_record" "ses_dkim" {
  count   = 3
  zone_id = data.aws_route53_zone.hosted_zone.zone_id
  name    = "${aws_sesv2_email_identity.domain.dkim_signing_attributes[0].tokens[count.index]}._domainkey"
  type    = "CNAME"
  ttl     = 600
  records = ["${aws_sesv2_email_identity.domain.dkim_signing_attributes[0].tokens[count.index]}.dkim.amazonses.com"]
}

# Report-only for now; switch to p=reject once codes are arriving fine.
resource "aws_route53_record" "dmarc" {
  zone_id = data.aws_route53_zone.hosted_zone.zone_id
  name    = "_dmarc"
  type    = "TXT"
  ttl     = 600
  records = ["v=DMARC1; p=none;"]
}

resource "aws_cognito_user_pool" "skipli" {
  region              = "us-east-1"
  name                = "skipli"
  user_pool_tier      = "ESSENTIALS" # required for passwordless sign-in
  username_attributes = ["email"]

  username_configuration {
    case_sensitive = false
  }

  # Users are only created by the API, on their first sign-in.
  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  sign_in_policy {
    allowed_first_auth_factors = ["EMAIL_OTP", "PASSWORD"]
  }

  email_configuration {
    email_sending_account = "DEVELOPER"
    source_arn            = aws_sesv2_email_identity.domain.arn
    from_email_address    = "Skipli <no-reply@${data.aws_route53_zone.hosted_zone.name}>"
  }
}

resource "aws_cognito_user_pool_client" "skipli" {
  region       = "us-east-1"
  name         = "skipli"
  user_pool_id = aws_cognito_user_pool.skipli.id

  explicit_auth_flows   = ["ALLOW_USER_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"]
  id_token_validity     = 24 # hours, same as the old JWT
  auth_session_validity = 10 # minutes to enter the emailed code
}
