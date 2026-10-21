package server

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/card"
	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/profile"
	"github.com/Schiffer116/skipli/api/internal/task"
)

// NewRouter serves the HTTP API: page loads, board-list changes, sign-in,
// invites and profiles. Board changes go to the realtime API instead.
func NewRouter(c config.Config) http.Handler {
	idp := cognitoidentityprovider.NewFromConfig(c.AWS, func(o *cognitoidentityprovider.Options) {
		o.Region = c.PoolRegion
	})
	return newRouter(c.DB, idp, c.Tokens, c.PoolID, c.ClientID, c.Table, auth.CanReceiveMail)
}

// newRouter wires the routes onto already-built clients, so tests can run the
// real router against fakes. canReceiveMail stands in for the invite
// address's DNS check.
func newRouter(db *dynamodb.Client, idp *cognitoidentityprovider.Client, tokens *auth.TokenVerifier, poolID, clientID, tableName string, canReceiveMail func(context.Context, string) bool) http.Handler {
	router := http.NewServeMux()

	users := auth.NewDirectory(idp, poolID)

	profiles := profile.NewStore(db, tableName)
	profile.NewHandler(profiles, tokens).RegisterRoutes(router)
	board.NewHandler(db, tableName, tokens, users, profiles, canReceiveMail).RegisterRoutes(router)
	auth.NewHandler(idp, clientID, users, tokens, auth.NewCooldown(db, tableName)).RegisterRoutes(router)
	card.NewHandler(db, tableName, tokens).RegisterRoutes(router)
	task.NewHandler(db, tableName, tokens).RegisterRoutes(router)

	api := http.NewServeMux()
	api.Handle("/api/", http.StripPrefix("/api", router))
	return withCORS(api)
}
