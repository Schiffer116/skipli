//go:build !lambda

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/realtime"
)

// serve listens on PORT, serving the board sockets the realtime Lambda
// serves in AWS too, so a developer starts one thing.
func serve(c config.Config, router http.Handler) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ws", realtime.NewHandler(c.AWS, c.DB, c.Table, c.Tokens).ServeLocal)
	mux.Handle("/", router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
