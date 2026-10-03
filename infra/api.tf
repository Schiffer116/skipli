variable "gmail" {
  type = string
}

variable "gmail_app_password" {
  type      = string
  sensitive = true
}

resource "random_password" "jwt" {
  length  = 64
  special = false
}

data "aws_iam_policy_document" "assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    principals {
      type        = "AWS"
      identifiers = [data.aws_caller_identity.current.arn]
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
      "arn:aws:dynamodb:ap-southeast-7:797848269974:table/Skipli",
      "arn:aws:dynamodb:ap-southeast-7:797848269974:table/Skipli/*"
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

  environment {
    variables = {
      APP_EMAIL          = var.gmail
      APP_EMAIL_PASSWORD = var.gmail_app_password
      JWT_SECRET         = random_password.jwt.result
    }
  }
}

resource "aws_apigatewayv2_api" "skipli" {
  name          = "skipli"
  protocol_type = "HTTP"
  target        = aws_lambda_function.skipli.arn
}

resource "aws_lambda_permission" "apigw" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.skipli.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.skipli.execution_arn}/*/*"
}
