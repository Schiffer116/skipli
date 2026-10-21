//go:build lambda

package main

import (
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/Schiffer116/skipli/api/internal/config"
)

// serve answers API Gateway HTTP API requests, which arrive as v2 payloads.
func serve(_ config.Config, router http.Handler) {
	lambda.Start(httpadapter.NewV2(router).ProxyWithContext)
}
