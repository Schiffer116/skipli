data "aws_iam_policy_document" "origin_bucket_policy" {
  statement {
    sid    = "AllowCloudFrontServicePrincipalReadWrite"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }

    actions = ["s3:GetObject"]

    resources = [
      "${aws_s3_bucket.skipli.arn}/ui/*",
    ]

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.skipli.arn]
    }
  }
}

resource "aws_s3_bucket_policy" "b" {
  region = local.state_region
  bucket = aws_s3_bucket.skipli.id
  policy = data.aws_iam_policy_document.origin_bucket_policy.json
}

locals {
  s3_origin_id    = "S3Origin"
  apigw_origin_id = "APIGatewayOrigin"
}

variable "ui_domain" {
  type = string
}

resource "aws_acm_certificate" "ui_domain" {
  region            = "us-east-1"
  domain_name       = var.ui_domain
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

data "aws_route53_zone" "hosted_zone" {
  name         = "schifferarchitecture.com"
  private_zone = false
}

resource "aws_route53_record" "domain_record" {
  for_each = {
    for dvo in aws_acm_certificate.ui_domain.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.hosted_zone.zone_id
}

resource "aws_acm_certificate_validation" "certificate_validation" {
  region                  = "us-east-1"
  certificate_arn         = aws_acm_certificate.ui_domain.arn
  validation_record_fqdns = [for record in aws_route53_record.domain_record : record.fqdn]
}

resource "aws_cloudfront_origin_access_control" "default" {
  name                              = "default-oac"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_function" "redirect_to_index" {
  name    = "RedirectToIndex"
  runtime = "cloudfront-js-2.0"
  publish = true
  code    = <<-EOF
    function handler(event) {
      var req = event.request;
      if (req.uri.indexOf('.') === -1) req.uri = '/index.html';
      return req;
    }
  EOF
}

resource "aws_cloudfront_distribution" "skipli" {
  origin {
    domain_name              = aws_s3_bucket.skipli.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.default.id
    origin_id                = local.s3_origin_id
    origin_path              = "/ui"
  }

  origin {
    domain_name = trimprefix(aws_apigatewayv2_api.skipli.api_endpoint, "https://")
    origin_id   = local.apigw_origin_id

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  enabled             = true
  is_ipv6_enabled     = true
  default_root_object = "index.html"

  aliases = [var.ui_domain]

  default_cache_behavior {
    allowed_methods  = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods   = ["GET", "HEAD"]
    target_origin_id = local.s3_origin_id

    forwarded_values {
      query_string = false

      cookies {
        forward = "none"
      }
    }

    viewer_protocol_policy = "allow-all"
    min_ttl                = 0
    default_ttl            = 3600
    max_ttl                = 86400

    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.redirect_to_index.arn
    }
  }

  ordered_cache_behavior {
    path_pattern     = "/api/*"
    allowed_methods  = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods   = ["GET", "HEAD"]
    target_origin_id = local.apigw_origin_id

    viewer_protocol_policy   = "redirect-to-https"
    cache_policy_id          = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad" # CachingDisabled
    origin_request_policy_id = "b689b0a8-53d0-40ab-baf2-68738e2966ac" # AllViewerExceptHostHeader
  }

  price_class = "PriceClass_200"

  restrictions {
    geo_restriction {
      restriction_type = "none"
      locations        = []
    }
  }

  tags = {
    Environment = "production"
  }

  viewer_certificate {
    acm_certificate_arn = aws_acm_certificate_validation.certificate_validation.certificate_arn
    ssl_support_method  = "sni-only"
  }
}

resource "aws_route53_record" "cloudfront" {
  for_each = aws_cloudfront_distribution.skipli.aliases
  zone_id  = data.aws_route53_zone.hosted_zone.zone_id
  name     = each.value
  type     = "A"

  alias {
    name                   = aws_cloudfront_distribution.skipli.domain_name
    zone_id                = aws_cloudfront_distribution.skipli.hosted_zone_id
    evaluate_target_health = false
  }
}
