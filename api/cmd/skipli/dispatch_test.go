package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestDispatchSendsEachEventToItsAPI(t *testing.T) {
	router := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(req.Method + " " + req.URL.Path))
	})
	var sockets []events.APIGatewayWebsocketProxyRequest
	handle := dispatch(router, func(_ context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
		sockets = append(sockets, req)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: "ack"}, nil
	})

	out, err := handle(context.Background(), json.RawMessage(`{
		"version": "2.0", "rawPath": "/api/boards",
		"requestContext": {"http": {"method": "GET", "path": "/api/boards"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if res := out.(events.APIGatewayV2HTTPResponse); res.StatusCode != http.StatusTeapot || res.Body != "GET /api/boards" {
		t.Errorf("HTTP event answered %d %q", res.StatusCode, res.Body)
	}
	if len(sockets) != 0 {
		t.Errorf("HTTP event reached the socket handler")
	}

	out, err = handle(context.Background(), json.RawMessage(`{
		"body": "{\"action\":\"ping\"}",
		"requestContext": {"routeKey": "$default", "eventType": "MESSAGE", "connectionId": "c1", "stage": "ws"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if res := out.(events.APIGatewayProxyResponse); res.Body != "ack" {
		t.Errorf("socket event answered %+v", res)
	}
	if len(sockets) != 1 || sockets[0].RequestContext.ConnectionID != "c1" || sockets[0].Body != `{"action":"ping"}` {
		t.Errorf("socket handler got %+v", sockets)
	}

	if _, err := handle(context.Background(), json.RawMessage(`{"source":"aws.events"}`)); err == nil {
		t.Error("unknown event accepted")
	}
}
