package realtime

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/coder/websocket"

	"github.com/Schiffer116/skipli/api/internal/access"
)

// ServeLocal stands in for API Gateway on a developer's machine: the same
// board socket, with its connections kept in memory instead of the table.
func (h *Handler) ServeLocal(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	user, err := h.tokens.VerifyCookies(req.Header.Get("Cookie"))
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	boardID := req.URL.Query().Get("boardId")
	allowed, err := access.Check(ctx, h.db, h.table, boardID, user.ID)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if allowed != http.StatusOK {
		http.Error(w, http.StatusText(allowed), allowed)
		return
	}

	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{OriginPatterns: []string{"localhost:*"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	h.mu.Lock()
	if h.rooms[boardID] == nil {
		h.rooms[boardID] = map[*websocket.Conn]struct{}{}
	}
	h.rooms[boardID][conn] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.rooms[boardID], conn)
		h.mu.Unlock()
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		reply, accepted := h.receive(ctx, data, func(changed string) (int, error) {
			if changed != boardID {
				return http.StatusForbidden, nil
			}
			return access.Check(ctx, h.db, h.table, boardID, user.ID)
		})
		if accepted != nil {
			h.broadcastLocal(req, conn, accepted)
		}
		if reply != nil {
			conn.Write(ctx, websocket.MessageText, reply)
		}
	}
}

func (h *Handler) broadcastLocal(req *http.Request, sender *websocket.Conn, accepted *broadcast) {
	data, err := json.Marshal(accepted)
	if err != nil {
		log.Printf("failed to encode change: %v", err)
		return
	}

	h.mu.Lock()
	var peers []*websocket.Conn
	for peer := range h.rooms[accepted.Change.BoardID] {
		if peer != sender {
			peers = append(peers, peer)
		}
	}
	h.mu.Unlock()

	for _, peer := range peers {
		peer.Write(req.Context(), websocket.MessageText, data)
	}
}
