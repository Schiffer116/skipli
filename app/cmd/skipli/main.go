package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/Schiffer116/skipli/internal/server"
)

func main() {
	server := server.NewServer()

	// On Lambda (behind a function URL, which sends API Gateway v2 payloads)
	// serve the same router through the adapter instead of a port.
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(httpadapter.NewV2(server.Router).ProxyWithContext)
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, server.Router))
}
