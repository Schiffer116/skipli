package realtime

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Schiffer116/skipli/api/internal/access"
)

// Limits are API Gateway's WebSocket limits, which the local relay enforces
// too so the app's reconnecting is exercised the same way.
type Limits struct {
	// Idle closes a connection the app hasn't sent anything on for this long.
	Idle time.Duration
	// Lifetime closes every connection this long after it opened.
	Lifetime time.Duration
	// Frame is the largest message the app may send, in bytes.
	Frame int64
}

var GatewayLimits = Limits{Idle: 10 * time.Minute, Lifetime: connectionLifetime, Frame: 128 << 10}

// ServeLocal stands in for API Gateway on a developer's machine: the same
// board socket, with its connections kept in memory instead of the table,
// and closed at API Gateway's limits.
func (h *Handler) ServeLocal(limits Limits) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) { h.serveLocal(w, req, limits) }
}

func (h *Handler) serveLocal(w http.ResponseWriter, req *http.Request, limits Limits) {
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
	conn.SetReadLimit(limits.Frame)

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

	lifetime, cancel := context.WithTimeout(ctx, limits.Lifetime)
	defer cancel()
	for {
		// A read that times out closes the connection, as API Gateway does
		// once the connection is idle or at the end of its lifetime. What
		// the app sends is only a keepalive, so it's read and dropped.
		read, cancelRead := context.WithTimeout(lifetime, limits.Idle)
		_, _, err := conn.Read(read)
		cancelRead()
		if err != nil {
			if errors.Is(lifetime.Err(), context.DeadlineExceeded) {
				log.Printf("closed a board socket at the end of its %v lifetime", limits.Lifetime)
			} else if errors.Is(read.Err(), context.DeadlineExceeded) {
				log.Printf("closed a board socket idle for %v", limits.Idle)
			}
			return
		}
	}
}

func (h *Handler) broadcastLocal(ctx context.Context, boardID string, data []byte) {
	h.mu.Lock()
	peers := make([]*websocket.Conn, 0, len(h.rooms[boardID]))
	for peer := range h.rooms[boardID] {
		peers = append(peers, peer)
	}
	h.mu.Unlock()

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peer.Write(ctx, websocket.MessageText, data)
		}()
	}
	wg.Wait()
}
