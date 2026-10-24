# Abuse limits for the public API. API Gateway throttles are account-wide per
# route rather than per client, and reserved concurrency caps how many copies of
# the Lambda can run at once, which bounds cost if the API is flooded.
variable "api_throttle_rate_limit" {
  description = "Steady-state requests per second allowed across every HTTP API route."
  type        = number
  default     = 25
}

variable "api_throttle_burst_limit" {
  description = "Burst of requests allowed across every HTTP API route."
  type        = number
  default     = 50
}

variable "auth_send_throttle_rate_limit" {
  description = "Steady-state requests per second allowed on POST /api/auth/send, which sends a sign-in email."
  type        = number
  default     = 1
}

variable "auth_send_throttle_burst_limit" {
  description = "Burst of requests allowed on POST /api/auth/send."
  type        = number
  default     = 5
}

variable "sort_throttle_rate_limit" {
  description = "Steady-state requests per second allowed on POST /api/boards/{boardId}/cards/{cardId}/sort, which asks Jev (a paid API)."
  type        = number
  default     = 2
}

variable "sort_throttle_burst_limit" {
  description = "Burst of requests allowed on the sort route."
  type        = number
  default     = 5
}

variable "jev_base_url" {
  description = "Where the API asks Jev: TypeSafe itself, or a gateway taking the same requests such as https://openrouter.ai/api. Empty means TypeSafe."
  type        = string
  default     = ""
}

variable "jev_model" {
  description = "The Jev model sorting uses, named as jev_base_url names it. Empty means the API's pinned default."
  type        = string
  default     = ""
}

variable "api_reserved_concurrency" {
  description = "Maximum concurrent executions of the API Lambda, which serves both the HTTP and the WebSocket API."
  type        = number
  default     = null
}

resource "aws_dynamodb_table" "skipli" {
  name         = "Skipli"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  attribute {
    name = "Member"
    type = "S"
  }

  global_secondary_index {
    name = "Member"

    key_schema {
      attribute_name = "Member"
      key_type       = "HASH"
    }

    key_schema {
      attribute_name = "PK"
      key_type       = "RANGE"
    }

    projection_type = "KEYS_ONLY"

    on_demand_throughput {
      max_read_request_units  = 5
      max_write_request_units = 5
    }
  }

  on_demand_throughput {
    max_read_request_units  = 5
    max_write_request_units = 5
  }

  point_in_time_recovery {
    enabled = true
  }

  ttl {
    attribute_name = "ExpiresAt"
    enabled        = true
  }

  lifecycle {
    prevent_destroy = true
  }
}

data "aws_iam_policy_document" "assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    dynamic "principals" {
      for_each = length(var.local_dev_principals) > 0 ? [1] : []
      content {
        type        = "AWS"
        identifiers = var.local_dev_principals
      }
    }

    actions = ["sts:AssumeRole"]
  }
}

data "aws_iam_policy_document" "dynamodb_access" {
  statement {
    effect = "Allow"

    actions = [
      "dynamodb:BatchGetItem",
      "dynamodb:BatchWriteItem",
      "dynamodb:ConditionCheckItem",
      "dynamodb:DeleteItem",
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:Query"
    ]

    resources = [
      aws_dynamodb_table.skipli.arn,
      "${aws_dynamodb_table.skipli.arn}/*"
    ]
  }
}

resource "aws_iam_role" "lambda" {
  name               = "SkipliLambdaExecutionRole"
  assume_role_policy = data.aws_iam_policy_document.assume_role.json
}

resource "aws_iam_role_policy_attachment" "logs" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "dynamodb_access" {
  name   = "SkipliDynamoDBAccess"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.dynamodb_access.json
}

data "aws_iam_policy_document" "cognito_access" {
  statement {
    effect    = "Allow"
    actions   = ["cognito-idp:AdminCreateUser", "cognito-idp:AdminSetUserPassword", "cognito-idp:AdminGetUser"]
    resources = [aws_cognito_user_pool.skipli.arn]
  }
}

resource "aws_iam_role_policy" "cognito_access" {
  name   = "SkipliCognitoAccess"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.cognito_access.json
}

data "aws_iam_policy_document" "realtime_access" {
  statement {
    effect    = "Allow"
    actions   = ["execute-api:ManageConnections"]
    resources = ["${aws_apigatewayv2_api.realtime.execution_arn}/*"]
  }
}

# The Jev key is an SSM SecureString (encrypted with the AWS managed aws/ssm
# key) that Terraform never sees: set it once per environment with
#   aws ssm put-parameter --type SecureString --name /skipli/typesafe-api-key --value <key>
# Until it is, sorting answers 503.
locals {
  jev_key_parameter = "/skipli/typesafe-api-key"
}

data "aws_iam_policy_document" "jev_key_access" {
  statement {
    effect    = "Allow"
    actions   = ["ssm:GetParameter"]
    resources = ["arn:aws:ssm:${var.region}:${data.aws_caller_identity.current.account_id}:parameter${local.jev_key_parameter}"]
  }
}

resource "aws_iam_role_policy" "jev_key_access" {
  name   = "SkipliJevKeyAccess"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.jev_key_access.json
}

resource "aws_iam_role_policy" "realtime_access" {
  name   = "SkipliRealtimeAccess"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.realtime_access.json
}

data "archive_file" "zip_archive" {
  type        = "zip"
  source_file = "${path.module}/../api/bin/bootstrap"
  output_path = "${path.module}/../api/bin/api.zip"
}

# One function serves both the HTTP API and the WebSocket API; it tells their
# events apart by shape (api/cmd/skipli).
resource "aws_lambda_function" "skipli" {
  filename      = data.archive_file.zip_archive.output_path
  function_name = "SkipliAPI"
  role          = aws_iam_role.lambda.arn

  handler          = "foo"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = data.archive_file.zip_archive.output_base64sha256

  reserved_concurrent_executions = var.api_reserved_concurrency

  # CPU comes with memory: at the default 128 MB a cold start and each new TLS
  # connection to DynamoDB and Cognito crawl, and opening a board or making a
  # Board change waits on it.
  memory_size = 512

  # Sorting a card waits on Jev, which has its own 10 s deadline; the HTTP
  # API gives up at 30 s anyway.
  timeout = 15

  environment {
    variables = {
      USER_POOL_ID        = aws_cognito_user_pool.skipli.id
      USER_POOL_CLIENT_ID = aws_cognito_user_pool_client.skipli.id
      TABLE_NAME          = aws_dynamodb_table.skipli.name
      # Where the HTTP handlers push what they save to a board's WebSockets.
      REALTIME_ENDPOINT = replace(aws_apigatewayv2_stage.realtime.invoke_url, "wss://", "https://")
      JEV_KEY_PARAMETER = local.jev_key_parameter
      JEV_BASE_URL      = var.jev_base_url
      JEV_MODEL         = var.jev_model
    }
  }
}

# The HTTP API is declared piece by piece rather than through quick create
# (target), because AWS does not let Terraform manage a quick-created $default
# stage, and that stage is where the throttling lives. Replacing the API gives it
# a new id and hostname; create_before_destroy keeps the old one serving until
# CloudFront points at the new one.
resource "aws_apigatewayv2_api" "skipli" {
  name          = "skipli"
  protocol_type = "HTTP"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_apigatewayv2_integration" "skipli" {
  api_id                 = aws_apigatewayv2_api.skipli.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.skipli.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "default" {
  api_id    = aws_apigatewayv2_api.skipli.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.skipli.id}"
}

# Per-route throttling needs a route of its own, so POST /api/auth/send gets one
# pointing at the same Lambda; every other request still falls to $default.
resource "aws_apigatewayv2_route" "auth_send" {
  api_id    = aws_apigatewayv2_api.skipli.id
  route_key = "POST /api/auth/send"
  target    = "integrations/${aws_apigatewayv2_integration.skipli.id}"
}

# Sorting asks Jev, which is billed per request, so it gets a route and a
# throttle of its own too.
resource "aws_apigatewayv2_route" "sort" {
  api_id    = aws_apigatewayv2_api.skipli.id
  route_key = "POST /api/boards/{boardId}/cards/{cardId}/sort"
  target    = "integrations/${aws_apigatewayv2_integration.skipli.id}"
}

resource "aws_apigatewayv2_stage" "skipli" {
  api_id      = aws_apigatewayv2_api.skipli.id
  name        = "$default"
  auto_deploy = true

  default_route_settings {
    throttling_rate_limit  = var.api_throttle_rate_limit
    throttling_burst_limit = var.api_throttle_burst_limit
  }

  route_settings {
    route_key              = aws_apigatewayv2_route.auth_send.route_key
    throttling_rate_limit  = var.auth_send_throttle_rate_limit
    throttling_burst_limit = var.auth_send_throttle_burst_limit
  }

  route_settings {
    route_key              = aws_apigatewayv2_route.sort.route_key
    throttling_rate_limit  = var.sort_throttle_rate_limit
    throttling_burst_limit = var.sort_throttle_burst_limit
  }
}

resource "aws_lambda_permission" "apigw" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.skipli.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.skipli.execution_arn}/*/*"

  # The old API keeps invoking the Lambda until CloudFront moves off it.
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_apigatewayv2_api" "realtime" {
  name                       = "skipli-realtime"
  protocol_type              = "WEBSOCKET"
  route_selection_expression = "$request.body.action"
}

resource "aws_apigatewayv2_integration" "realtime" {
  api_id             = aws_apigatewayv2_api.realtime.id
  integration_type   = "AWS_PROXY"
  integration_method = "POST"
  integration_uri    = aws_lambda_function.skipli.invoke_arn
}

# The socket carries news out only: members connect and disconnect, and the app
# pings on $default to keep an idle connection open. Changes are HTTP requests.
resource "aws_apigatewayv2_route" "realtime" {
  for_each  = toset(["$connect", "$disconnect", "$default"])
  api_id    = aws_apigatewayv2_api.realtime.id
  route_key = each.key
  target    = "integrations/${aws_apigatewayv2_integration.realtime.id}"
}

resource "aws_apigatewayv2_stage" "realtime" {
  api_id      = aws_apigatewayv2_api.realtime.id
  name        = "ws"
  auto_deploy = true
}

resource "aws_lambda_permission" "realtime" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.skipli.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.realtime.execution_arn}/*"
}
