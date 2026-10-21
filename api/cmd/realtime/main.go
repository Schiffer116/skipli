// Command realtime is the board WebSocket API: a Lambda behind API Gateway.
// Locally, cmd/http serves the same sockets.
package main

import (
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/realtime"
)

func main() {
	c := config.Load()
	lambda.Start(realtime.NewHandler(c.AWS, c.DB, c.Table, c.Tokens).Handle)
}
