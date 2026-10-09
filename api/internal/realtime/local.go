package realtime

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/coder/websocket"
)

func (h *Handler) ServeLocal(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	user, err := h.tokens.VerifyCookies(req.Header.Get("Cookie"))
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	boardID := req.URL.Query().Get("boardId")
	member, err := h.exists(ctx, boardID, "MEMBER#"+user.ID)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !member {
		http.Error(w, "not a member of this board", http.StatusForbidden)
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

		var msg message
		if err := json.Unmarshal(data, &msg); err != nil || msg.Event == "" {
			continue
		}
		out, err := json.Marshal(struct {
			Event string          `json:"event"`
			Args  json.RawMessage `json:"args"`
		}{msg.Event, msg.Args})
		if err != nil {
			continue
		}

		h.mu.Lock()
		var peers []*websocket.Conn
		for peer := range h.rooms[boardID] {
			if peer != conn {
				peers = append(peers, peer)
			}
		}
		h.mu.Unlock()

		for _, peer := range peers {
			peer.Write(ctx, websocket.MessageText, out)
		}
	}
}
