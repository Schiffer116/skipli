resource "aws_sesv2_email_identity" "domain" {
  region         = "us-east-1"
  email_identity = data.aws_route53_zone.hosted_zone.name
}

resource "aws_route53_record" "ses_dkim" {
  provider = aws.dns
  count    = 3
  zone_id  = data.aws_route53_zone.hosted_zone.zone_id
  name     = "${aws_sesv2_email_identity.domain.dkim_signing_attributes[0].tokens[count.index]}._domainkey"
  type     = "CNAME"
  ttl      = 600
  records  = ["${aws_sesv2_email_identity.domain.dkim_signing_attributes[0].tokens[count.index]}.dkim.amazonses.com"]
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
    from_email_address    = "${var.email_from_name} <no-reply@${data.aws_route53_zone.hosted_zone.name}>"
  }

  lambda_config {
    pre_sign_up = aws_lambda_function.pre_sign_up.arn
  }
}

resource "aws_cognito_user_pool_client" "skipli" {
  region       = "us-east-1"
  name         = "skipli"
  user_pool_id = aws_cognito_user_pool.skipli.id

  explicit_auth_flows   = ["ALLOW_USER_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"]
  id_token_validity     = 1
  auth_session_validity = 10

  # Google sign-in: the API's callback is the only redirect target.
  allowed_oauth_flows_user_pool_client = var.google_sign_in
  allowed_oauth_flows                  = var.google_sign_in ? ["code"] : []
  allowed_oauth_scopes                 = var.google_sign_in ? ["openid", "email", "profile"] : []
  supported_identity_providers         = aws_cognito_identity_provider.google[*].provider_name
  callback_urls                        = var.google_sign_in ? [for url in concat([local.app_url], var.local_app_urls) : "${url}/api/auth/google/callback"] : []
}

output "user_pool_id" {
  value = aws_cognito_user_pool.skipli.id
}

output "user_pool_client_id" {
  value = aws_cognito_user_pool_client.skipli.id
}
