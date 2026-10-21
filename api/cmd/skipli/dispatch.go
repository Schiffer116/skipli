package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

// dispatch sends each Lambda event to the API it came from, told apart by its
// shape: the WebSocket API's events name a connection, the HTTP API's (v2
// payloads) an HTTP method.
func dispatch(router http.Handler, socket func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error)) func(context.Context, json.RawMessage) (any, error) {
	httpAPI := httpadapter.NewV2(router)
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var shape struct {
			RequestContext struct {
				ConnectionID string `json:"connectionId"`
				HTTP         *struct {
					Method string `json:"method"`
				} `json:"http"`
			} `json:"requestContext"`
		}
		if err := json.Unmarshal(raw, &shape); err != nil {
			return nil, err
		}

		switch {
		case shape.RequestContext.ConnectionID != "":
			var req events.APIGatewayWebsocketProxyRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, err
			}
			return socket(ctx, req)
		case shape.RequestContext.HTTP != nil:
			var req events.APIGatewayV2HTTPRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, err
			}
			return httpAPI.ProxyWithContext(ctx, req)
		}
		return nil, fmt.Errorf("unrecognised event")
	}
}
