# Google sign-in through the user pool's hosted OAuth endpoints. The API runs
# the authorization code + PKCE flow against the Cognito domain and sets the
# same session cookies as email codes. The Google OAuth client is made by hand
# in Google Cloud Console; its id and secret live in SSM Parameter Store in this
# environment's account (us-east-1) and are never committed.
variable "google_sign_in" {
  description = "Offer Google sign-in. Needs /skipli/google/client-id and /skipli/google/client-secret in SSM (us-east-1)."
  type        = bool
  default     = false
}

variable "auth_domain_prefix" {
  description = "Prefix of the user pool's Cognito domain, <prefix>.auth.us-east-1.amazoncognito.com. Globally unique."
  type        = string
}

variable "local_app_urls" {
  description = "Origins of locally run copies of the UI that may finish Google sign-in, such as http://localhost:4417."
  type        = list(string)
  default     = []
}

locals {
  google_count = var.google_sign_in ? 1 : 0
  app_url      = "https://${var.ui_domain}"
  auth_domain  = "https://${var.auth_domain_prefix}.auth.us-east-1.amazoncognito.com"
}

data "aws_ssm_parameter" "google_client_id" {
  count  = local.google_count
  region = "us-east-1"
  name   = "/skipli/google/client-id"
}

data "aws_ssm_parameter" "google_client_secret" {
  count  = local.google_count
  region = "us-east-1"
  name   = "/skipli/google/client-secret"
}

resource "aws_cognito_user_pool_domain" "skipli" {
  count                 = local.google_count
  region                = "us-east-1"
  domain                = var.auth_domain_prefix
  user_pool_id          = aws_cognito_user_pool.skipli.id
  managed_login_version = 1
}

resource "aws_cognito_identity_provider" "google" {
  count         = local.google_count
  region        = "us-east-1"
  user_pool_id  = aws_cognito_user_pool.skipli.id
  provider_name = "Google"
  provider_type = "Google"

  provider_details = {
    client_id        = data.aws_ssm_parameter.google_client_id[0].value
    client_secret    = data.aws_ssm_parameter.google_client_secret[0].value
    authorize_scopes = "openid email profile"
  }

  attribute_mapping = {
    email          = "email"
    email_verified = "email_verified"
    username       = "sub"
  }
}

# Cognito doesn't link a Google sign-in to an existing account with the same
# email on its own, so this trigger does it before the federated user would be
# created (see SignUpWithProvider in api/internal/auth). Triggers must run in
# the user pool's region.
resource "aws_lambda_function" "pre_sign_up" {
  region        = "us-east-1"
  filename      = data.archive_file.zip_archive.output_path
  function_name = "SkipliPreSignUp"
  role          = aws_iam_role.pre_sign_up.arn

  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = data.archive_file.zip_archive.output_base64sha256

  environment {
    variables = {
      COGNITO_TRIGGER = "1"
    }
  }
}

resource "aws_lambda_permission" "pre_sign_up" {
  region        = "us-east-1"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.pre_sign_up.function_name
  principal     = "cognito-idp.amazonaws.com"
  source_arn    = aws_cognito_user_pool.skipli.arn
}

data "aws_iam_policy_document" "pre_sign_up_assume_role" {
  statement {
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "pre_sign_up" {
  name               = "SkipliPreSignUpRole"
  assume_role_policy = data.aws_iam_policy_document.pre_sign_up_assume_role.json
}

resource "aws_iam_role_policy_attachment" "pre_sign_up_logs" {
  role       = aws_iam_role.pre_sign_up.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_iam_policy_document" "pre_sign_up" {
  statement {
    actions = [
      "cognito-idp:AdminCreateUser",
      "cognito-idp:AdminGetUser",
      "cognito-idp:AdminLinkProviderForUser",
      "cognito-idp:AdminSetUserPassword",
    ]
    resources = [aws_cognito_user_pool.skipli.arn]
  }
}

resource "aws_iam_role_policy" "pre_sign_up" {
  name   = "SkipliPreSignUpCognitoAccess"
  role   = aws_iam_role.pre_sign_up.id
  policy = data.aws_iam_policy_document.pre_sign_up.json
}

output "auth_domain" {
  value = var.google_sign_in ? local.auth_domain : ""
}
