package server

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/internal/auth"
	"github.com/Schiffer116/skipli/internal/board"
	"github.com/Schiffer116/skipli/internal/card"
	"github.com/Schiffer116/skipli/internal/task"
)

type Server struct {
	Router http.Handler
}

func NewServer() *Server {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("ap-southeast-7"))
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}

	appEmail := os.Getenv("APP_EMAIL")
	appEmailPassword := os.Getenv("APP_EMAIL_PASSWORD")
	jwtSecret := os.Getenv("JWT_SECRET")
	if appEmail == "" || appEmailPassword == "" {
		log.Fatal("APP_EMAIL and APP_EMAIL_PASSWORD must be set")
	}
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	db := dynamodb.NewFromConfig(cfg)
	mailer := auth.NewMailer(appEmail, appEmailPassword)
	tokens := auth.NewTokenIssuer(jwtSecret)
	router := http.NewServeMux()

	board.NewHandler(db, tokens).RegisterRoutes(router)
	auth.NewHandler(db, mailer, tokens).RegisterRoutes(router)
	card.NewHandler(db).RegisterRoutes(router)
	task.NewHandler(db).RegisterRoutes(router)

	api := http.NewServeMux()
	api.Handle("/api/", http.StripPrefix("/api", router))

	return &Server{Router: withCORS(api)}
}
