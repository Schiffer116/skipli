// Command skipli is the whole API: one Lambda behind both the HTTP API and the
// board WebSocket API, or, on a developer's machine, one server for both.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/realtime"
	"github.com/Schiffer116/skipli/api/internal/server"
)

func main() {
	c := config.Load()
	router := server.NewRouter(c)
	rt := realtime.NewHandler(c.AWS, c.DB, c.Table, c.Tokens)

	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(dispatch(router, rt.Handle))
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ws", rt.ServeLocal(socketLimits()))
	mux.Handle("/", router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// socketLimits are API Gateway's, unless WS_IDLE or WS_LIFETIME (such as
// "30s") shorten them to try reconnecting without waiting.
func socketLimits() realtime.Limits {
	limits := realtime.GatewayLimits
	for name, limit := range map[string]*time.Duration{"WS_IDLE": &limits.Idle, "WS_LIFETIME": &limits.Lifetime} {
		if value := os.Getenv(name); value != "" {
			d, err := time.ParseDuration(value)
			if err != nil {
				log.Fatalf("%s: %v", name, err)
			}
			*limit = d
		}
	}
	return limits
}
