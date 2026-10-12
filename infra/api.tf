# Abuse limits for the public API. API Gateway throttles are account-wide per
# route rather than per client, and reserved concurrency caps how many copies of
# each Lambda can run at once, which bounds cost if the API is flooded.
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

variable "api_reserved_concurrency" {
  description = "Maximum concurrent executions of the HTTP API Lambda."
  type        = number
  default     = 10
}

variable "realtime_reserved_concurrency" {
  description = "Maximum concurrent executions of the WebSocket Lambda."
  type        = number
  default     = 10
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

resource "aws_lambda_function" "skipli" {
  filename      = data.archive_file.zip_archive.output_path
  function_name = "SkipliAPI"
  role          = aws_iam_role.lambda.arn

  handler          = "foo"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = data.archive_file.zip_archive.output_base64sha256

  reserved_concurrent_executions = var.api_reserved_concurrency

  environment {
    variables = {
      USER_POOL_ID        = aws_cognito_user_pool.skipli.id
      USER_POOL_CLIENT_ID = aws_cognito_user_pool_client.skipli.id
      TABLE_NAME          = aws_dynamodb_table.skipli.name
    }
  }
}

resource "aws_apigatewayv2_api" "skipli" {
  name          = "skipli"
  protocol_type = "HTTP"
  target        = aws_lambda_function.skipli.arn
}

# Quick create (target above) made the integration, the $default route and the
# auto-deployed $default stage. The stage is adopted here so it can carry
# throttling; dropping target instead would replace the whole API. On a brand
# new environment, apply aws_apigatewayv2_api.skipli on its own first so the
# stage exists to import.
import {
  to = aws_apigatewayv2_stage.skipli
  id = "${aws_apigatewayv2_api.skipli.id}/$default"
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
}

# Per-route throttling needs a route of its own, so POST /api/auth/send gets one
# pointing at the same Lambda; every other request still falls to $default.
resource "aws_apigatewayv2_integration" "skipli" {
  api_id                 = aws_apigatewayv2_api.skipli.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.skipli.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "auth_send" {
  api_id    = aws_apigatewayv2_api.skipli.id
  route_key = "POST /api/auth/send"
  target    = "integrations/${aws_apigatewayv2_integration.skipli.id}"
}

resource "aws_lambda_permission" "apigw" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.skipli.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.skipli.execution_arn}/*/*"
}

resource "aws_lambda_function" "realtime" {
  filename      = data.archive_file.zip_archive.output_path
  function_name = "SkipliRealtime"
  role          = aws_iam_role.lambda.arn

  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = data.archive_file.zip_archive.output_base64sha256

  reserved_concurrent_executions = var.realtime_reserved_concurrency

  environment {
    variables = {
      WEBSOCKET           = "1"
      USER_POOL_ID        = aws_cognito_user_pool.skipli.id
      USER_POOL_CLIENT_ID = aws_cognito_user_pool_client.skipli.id
      TABLE_NAME          = aws_dynamodb_table.skipli.name
    }
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
  integration_uri    = aws_lambda_function.realtime.invoke_arn
}

resource "aws_apigatewayv2_route" "realtime" {
  for_each  = toset(["$connect", "$default"])
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
  function_name = aws_lambda_function.realtime.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.realtime.execution_arn}/*"
}
