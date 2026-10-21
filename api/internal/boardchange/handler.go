package boardchange

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

// maxBody is far more than any Board change needs.
const maxBody = 128 << 10

// Handler takes Board changes over HTTP, and broadcasts the ones it accepts
// to the board's open connections before it answers.
type Handler struct {
	db          *dynamodb.Client
	table       string
	tokens      *auth.TokenVerifier
	store       *Store
	broadcaster broadcast.Broadcaster
}

func NewHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier, broadcaster broadcast.Broadcaster) *Handler {
	return &Handler{db: db, table: table, tokens: tokens, store: NewStore(db, table), broadcaster: broadcaster}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /boards/{boardId}/changes",
		board.RequireMember(h.db, h.table, h.tokens, "boardId", h.post))
}

// post applies the change in the body. A refusal answers with its status (see
// Refusal) so the app can tell a change the board can't take from a failure.
func (h *Handler) post(w http.ResponseWriter, req *http.Request) {
	var change Change
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBody)).Decode(&change); err != nil {
		http.Error(w, "not a change", http.StatusBadRequest)
		return
	}
	if change.BoardID != req.PathValue("boardId") {
		http.Error(w, "change is for another board", http.StatusBadRequest)
		return
	}

	if err := h.store.Apply(req.Context(), change); err != nil {
		status := Status(err)
		if status == http.StatusInternalServerError {
			log.Printf("failed to apply %s: %v", change.Type, err)
			http.Error(w, "failed to apply change", status)
			return
		}
		http.Error(w, err.Error(), status)
		return
	}

	h.broadcaster.Broadcast(req.Context(), change.BoardID, broadcast.Message{
		Type:   broadcast.Change,
		ID:     req.Header.Get(broadcast.ChangeIDHeader),
		Change: change,
	})
	w.WriteHeader(http.StatusNoContent)
}
