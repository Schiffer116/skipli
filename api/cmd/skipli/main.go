package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/server"
)

func main() {
	// The user pool's pre sign-up trigger shares this binary but none of the
	// API's setup: Cognito hands it the pool in every event.
	if os.Getenv("COGNITO_TRIGGER") != "" {
		lambda.Start(preSignUp())
		return
	}

	server := server.NewServer()

	// On Lambda (behind an API Gateway HTTP API, which sends v2 payloads)
	// serve the same router through the adapter instead of a port.
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		if os.Getenv("WEBSOCKET") != "" {
			lambda.Start(server.Realtime.Handle)
			return
		}
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

func preSignUp() func(context.Context, events.CognitoEventUserPoolsPreSignup) (events.CognitoEventUserPoolsPreSignup, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}
	idp := cognitoidentityprovider.NewFromConfig(cfg)
	return func(ctx context.Context, event events.CognitoEventUserPoolsPreSignup) (events.CognitoEventUserPoolsPreSignup, error) {
		return auth.PreSignUp(ctx, idp, event)
	}
}
