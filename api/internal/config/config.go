// Package config builds what both the HTTP and the realtime API start from:
// AWS clients, the table, and the user pool's token verifier.
package config

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/auth"
)

type Config struct {
	AWS      aws.Config
	DB       *dynamodb.Client
	Table    string
	PoolID   string
	ClientID string
	// PoolRegion is where the user pool lives, which may differ from the
	// API's own region.
	PoolRegion string
	Tokens     *auth.TokenVerifier
}

func Load() Config {
	ctx := context.Background()

	poolID := os.Getenv("USER_POOL_ID")
	clientID := os.Getenv("USER_POOL_CLIENT_ID")
	table := os.Getenv("TABLE_NAME")
	if poolID == "" || clientID == "" {
		log.Fatal("USER_POOL_ID and USER_POOL_CLIENT_ID must be set")
	}
	if table == "" {
		table = "Skipli"
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}

	poolRegion, _, _ := strings.Cut(poolID, "_")
	tokens, err := auth.NewTokenVerifier(ctx, poolRegion, poolID, clientID)
	if err != nil {
		log.Fatalf("unable to load user pool keys: %v", err)
	}

	return Config{
		AWS:        cfg,
		DB:         dynamodb.NewFromConfig(cfg),
		Table:      table,
		PoolID:     poolID,
		ClientID:   clientID,
		PoolRegion: poolRegion,
		Tokens:     tokens,
	}
}
