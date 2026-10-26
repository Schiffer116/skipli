package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// dial opens a board socket on the local relay as the board's member, and
// waits for the Presence the relay sends once it has joined the connection to
// the board: the handshake finishes before that, and a broadcast in between
// would miss it.
func (f *fixture) dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Cookie": {"id_token=" + f.token(t, "u-member", f.key)}, "Origin": {"http://localhost:5173"}}
	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(url, "http")+"?boardId=b1", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	if message, err := readAny(t, conn); err != nil || message["type"] != "presence" {
		t.Fatalf("got %v %v, want Presence on joining", message, err)
	}
	return conn
}

// read is the next message that isn't about Presence.
func read(t *testing.T, conn *websocket.Conn) (map[string]any, error) {
	t.Helper()
	for {
		message, err := readAny(t, conn)
		if err != nil || message["type"] != "presence" {
			return message, err
		}
	}
}

func readAny(t *testing.T, conn *websocket.Conn) (map[string]any, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var message map[string]any
	return message, json.Unmarshal(data, &message)
}

func TestLocalRelayDeliversBroadcastsToEveryConnectionOnTheBoard(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.handler.ServeLocal(GatewayLimits))
	t.Cleanup(srv.Close)
	first, second := f.dial(t, srv.URL), f.dial(t, srv.URL)

	f.handler.Broadcast(context.Background(), "b1", changed)
	for _, conn := range []*websocket.Conn{first, second} {
		if got, err := read(t, conn); err != nil || got["type"] != "change" || got["id"] != "x1" {
			t.Fatalf("got %v %v, want the change", got, err)
		}
	}
}

func TestLocalRelayClosesConnectionsAtAPIGatewaysLimits(t *testing.T) {
	f := setup(t)
	limits := Limits{Idle: 200 * time.Millisecond, Lifetime: time.Second, Frame: 1 << 10}
	srv := httptest.NewServer(f.handler.ServeLocal(limits))
	t.Cleanup(srv.Close)
	ping := func(conn *websocket.Conn) {
		conn.Write(context.Background(), websocket.MessageText, []byte(`{"action":"ping"}`))
	}
	pingUntil := func(conn *websocket.Conn, until time.Time) {
		for time.Now().Before(until) {
			time.Sleep(50 * time.Millisecond)
			ping(conn)
		}
	}

	idle, pinging := f.dial(t, srv.URL), f.dial(t, srv.URL)
	opened := time.Now()
	pingUntil(pinging, opened.Add(600*time.Millisecond))
	// API Gateway says so with 1001 Going Away, not by dropping the line.
	if _, err := read(t, idle); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("idle connection: got %v, want closed with 1001", err)
	}
	// Pings keep this one open past the idle limit, but not past its
	// lifetime.
	pingUntil(pinging, opened.Add(limits.Lifetime-150*time.Millisecond))
	f.handler.Broadcast(context.Background(), "b1", changed)
	if got, err := read(t, pinging); err != nil || got["type"] != "change" {
		t.Fatalf("pinged connection closed early: %v %v", got, err)
	}
	pingUntil(pinging, opened.Add(limits.Lifetime+200*time.Millisecond))
	if _, err := read(t, pinging); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("connection past its lifetime: got %v, want closed with 1001", err)
	}
}

func TestLocalRelayRefusesFramesOverTheLimit(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.handler.ServeLocal(Limits{Idle: time.Hour, Lifetime: time.Hour, Frame: 64}))
	t.Cleanup(srv.Close)
	conn := f.dial(t, srv.URL)
	conn.Write(context.Background(), websocket.MessageText, []byte(strings.Repeat("x", 100)))
	if _, err := read(t, conn); err == nil {
		t.Fatal("frame over the limit accepted")
	}
}

func TestLocalRelayTellsPresenceOnConnectAndDisconnect(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.handler.ServeLocal(GatewayLimits))
	t.Cleanup(srv.Close)
	names := func(message map[string]any) (ids []string) {
		for _, p := range message["people"].([]any) {
			ids = append(ids, p.(map[string]any)["id"].(string))
		}
		return ids
	}

	first := f.dial(t, srv.URL)
	// Two tabs of one person are one avatar.
	second := f.dial(t, srv.URL)
	got, err := readAny(t, first)
	if err != nil || got["type"] != "presence" {
		t.Fatalf("got %v %v, want presence", got, err)
	}
	if ids := names(got); !slices.Equal(ids, []string{"u-peer", "u-member"}) {
		t.Errorf("present %v, want the peer and the member (two tabs) once, by name", ids)
	}

	second.Close(websocket.StatusNormalClosure, "")
	if got, err := readAny(t, first); err != nil || got["type"] != "presence" {
		t.Fatalf("got %v %v, want presence after a tab closed", got, err)
	}
}
