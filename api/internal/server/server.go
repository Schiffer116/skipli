package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/card"
	"github.com/Schiffer116/skipli/api/internal/task"
)

type Server struct {
	Router http.Handler
}

func NewServer() *Server {
	ctx := context.Background()

	poolID := os.Getenv("USER_POOL_ID")
	clientID := os.Getenv("USER_POOL_CLIENT_ID")
	tableName := os.Getenv("TABLE_NAME")
	if poolID == "" || clientID == "" {
		log.Fatal("USER_POOL_ID and USER_POOL_CLIENT_ID must be set")
	}
	if tableName == "" {
		tableName = "Skipli"
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}
	db := dynamodb.NewFromConfig(cfg)

	poolRegion, _, _ := strings.Cut(poolID, "_")
	idp := cognitoidentityprovider.NewFromConfig(cfg, func(o *cognitoidentityprovider.Options) {
		o.Region = poolRegion
	})
	tokens, err := auth.NewTokenVerifier(ctx, poolRegion, poolID, clientID)
	if err != nil {
		log.Fatalf("unable to load user pool keys: %v", err)
	}
	router := http.NewServeMux()

	board.NewHandler(db, tableName, tokens).RegisterRoutes(router)
	auth.NewHandler(idp, poolID, clientID, tokens).RegisterRoutes(router)
	card.NewHandler(db, tableName).RegisterRoutes(router)
	task.NewHandler(db, tableName).RegisterRoutes(router)

	api := http.NewServeMux()
	api.Handle("/api/", http.StripPrefix("/api", router))

	return &Server{Router: withCORS(api)}
}
