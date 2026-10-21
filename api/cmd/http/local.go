//go:build !lambda

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/realtime"
)

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

// serve listens on PORT, serving the board sockets the realtime Lambda
// serves in AWS too, so a developer starts one thing.
func serve(c config.Config, router http.Handler) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ws", realtime.NewHandler(c.AWS, c.DB, c.Table, c.Tokens).ServeLocal(socketLimits()))
	mux.Handle("/", router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
