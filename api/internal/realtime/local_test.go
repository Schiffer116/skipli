package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// dial opens a board socket on the local relay as the board's member.
func (f *fixture) dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Cookie": {"id_token=" + f.token(t, "u-member", f.key)}, "Origin": {"http://localhost:5173"}}
	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(url, "http")+"?boardId=b1", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn) (map[string]any, error) {
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

func TestLocalRelayAcksTheSenderAndBroadcastsToPeers(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.handler.ServeLocal(GatewayLimits))
	t.Cleanup(srv.Close)
	sender, peer := f.dial(t, srv.URL), f.dial(t, srv.URL)

	change := `{"type":"deleteTask","boardId":"b1","cardId":"c1","taskId":"t1"}`
	if err := sender.Write(context.Background(), websocket.MessageText, []byte(changeMessage("m1", change))); err != nil {
		t.Fatal(err)
	}
	if got, err := read(t, sender); err != nil || got["type"] != "ack" || got["id"] != "m1" || got["status"] != 200.0 {
		t.Fatalf("sender got %v %v, want its ack", got, err)
	}
	if got, err := read(t, peer); err != nil || got["type"] != "change" {
		t.Fatalf("peer got %v %v, want the change", got, err)
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
	if _, err := read(t, idle); err == nil {
		t.Fatal("idle connection still open")
	}
	// Pings keep this one open past the idle limit, but not past its
	// lifetime.
	pingUntil(pinging, opened.Add(limits.Lifetime-150*time.Millisecond))
	pinging.Write(context.Background(), websocket.MessageText,
		[]byte(changeMessage("m", `{"type":"deleteTask","boardId":"b1","cardId":"c1","taskId":"t1"}`)))
	if got, err := read(t, pinging); err != nil || got["status"] != 200.0 {
		t.Fatalf("pinged connection closed early: %v %v", got, err)
	}
	pingUntil(pinging, opened.Add(limits.Lifetime+200*time.Millisecond))
	if _, err := read(t, pinging); err == nil {
		t.Fatal("connection open past its lifetime")
	}
}

func TestLocalRelayRefusesFramesOverTheLimit(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.handler.ServeLocal(Limits{Idle: time.Hour, Lifetime: time.Hour, Frame: 64}))
	t.Cleanup(srv.Close)
	conn := f.dial(t, srv.URL)
	conn.Write(context.Background(), websocket.MessageText, []byte(changeMessage("big", `{"type":"cleanUp","boardId":"b1"}`)))
	if _, err := read(t, conn); err == nil {
		t.Fatal("frame over the limit accepted")
	}
}
