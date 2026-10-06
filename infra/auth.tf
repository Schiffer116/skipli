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
  user_pool_tier      = "ESSENTIALS"
  username_attributes = ["email"]

  username_configuration {
    case_sensitive = false
  }

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
  id_token_validity     = 24
  auth_session_validity = 10
}

output "user_pool_id" {
  value = aws_cognito_user_pool.skipli.id
}

output "user_pool_client_id" {
  value = aws_cognito_user_pool_client.skipli.id
}
