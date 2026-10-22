package server

import (
	"encoding/json"
	"net/http"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// registerPresence serves who has a board open. The app reads it each time
// the board's socket opens, since the socket only tells of later changes.
func registerPresence(router *http.ServeMux, db *dynamodb.Client, table string, tokens *auth.TokenVerifier, roster broadcast.Roster) {
	router.HandleFunc("GET /boards/{id}/presence", board.RequireMember(db, table, tokens, "id", func(w http.ResponseWriter, req *http.Request) {
		people := roster.Present(req.Context(), req.PathValue("id"))
		if people == nil {
			people = []broadcast.Person{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(people)
	}))
}
