package realtime

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	gwtypes "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi/types"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

const (
	testIssuer   = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	testClientID = "test-client"
	newID        = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"
)

// The table: board b1 with its member and the connections open on it, and
// a connection on another board. conn-gone has closed.
var table = map[string]map[string]any{
	"b1/META":                {},
	"b1/MEMBER#u-member":     {},
	"CONNS#b1/conn-member":   {},
	"CONNS#b1/conn-peer":     {},
	"CONNS#b1/conn-gone":     {},
	"CONNS#b2/conn-other":    {},
	"CONN#conn-member/BOARD": {"Board": map[string]any{"S": "b1"}},
	"CONN#conn-orphan/BOARD": {},
}

func keyOf(body map[string]any) string {
	key, _ := body["Key"].(map[string]any)
	pk, _ := key["PK"].(map[string]any)
	sk, _ := key["SK"].(map[string]any)
	return fmt.Sprint(pk["S"], "/", sk["S"])
}

// fakeTable answers reads from table; every write succeeds.
func fakeTable(t *testing.T) *awsfake.Server {
	return awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "GetItem":
			if item, ok := table[keyOf(body)]; ok {
				return 200, map[string]any{"Item": item}
			}
			return 200, map[string]any{}
		case "Query":
			values, _ := json.Marshal(body["ExpressionAttributeValues"])
			var items []any
			for key := range table {
				pk, sk, _ := strings.Cut(key, "/")
				if strings.Contains(string(values), `"`+pk+`"`) && strings.HasPrefix(pk, "CONNS#") {
					items = append(items, map[string]any{"SK": map[string]any{"S": sk}})
				}
			}
			return 200, map[string]any{"Items": items}
		}
		return 200, map[string]any{}
	})
}

// fakeGateway records what reaches each connection; conn-gone has closed.
type fakeGateway struct {
	mu   sync.Mutex
	sent map[string][]string
	// fail makes pushes to these connections fail; block, when set, holds
	// every push until it is closed, after announcing it on arrived.
	fail    map[string]bool
	block   chan struct{}
	arrived chan string
}

func (g *fakeGateway) PostToConnection(_ context.Context, in *apigatewaymanagementapi.PostToConnectionInput, _ ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error) {
	if g.arrived != nil {
		g.arrived <- *in.ConnectionId
		<-g.block
	}
	if *in.ConnectionId == "conn-gone" {
		return nil, &gwtypes.GoneException{}
	}
	if g.fail[*in.ConnectionId] {
		return nil, errors.New("unreachable")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sent[*in.ConnectionId] = append(g.sent[*in.ConnectionId], string(in.Data))
	return &apigatewaymanagementapi.PostToConnectionOutput{}, nil
}

type fixture struct {
	db      *awsfake.Server
	gateway *fakeGateway
	handler *Handler
	key     *rsa.PrivateKey
}

// signingKey is made once: generating RSA keys is slow.
var signingKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

func setup(t *testing.T) *fixture {
	key := signingKey()
	tokens := auth.NewTokenVerifierWithKeyfunc(func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, testIssuer, testClientID)
	f := &fixture{db: fakeTable(t), gateway: &fakeGateway{sent: map[string][]string{}}, key: key}
	f.handler = newHandler(f.db.DynamoDB(), "Skipli", tokens, f.gateway)
	return f
}

var changed = broadcast.Message{
	Type: broadcast.Change,
	ID:   "x1",
	Change: map[string]any{
		"type": "updateTask", "boardId": "b1", "cardId": "c1", "taskId": "t1",
		"name": "Ship it", "description": "", "status": "done",
	},
}

func sameJSON(a, b string) bool {
	var x, y any
	json.Unmarshal([]byte(a), &x)
	json.Unmarshal([]byte(b), &y)
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

func TestBroadcastReachesEveryConnectionOnTheBoardAndForgetsClosedOnes(t *testing.T) {
	f := setup(t)
	f.handler.Broadcast(context.Background(), "b1", changed)

	want, _ := json.Marshal(changed)
	for _, conn := range []string{"conn-member", "conn-peer"} {
		if sent := f.gateway.sent[conn]; len(sent) != 1 || !sameJSON(sent[0], string(want)) {
			t.Errorf("%s received %v, want %s", conn, sent, want)
		}
	}
	if sent := f.gateway.sent["conn-other"]; len(sent) != 0 {
		t.Errorf("another board's connection received %v", sent)
	}
	deletes := f.db.Calls("DeleteItem")
	if len(deletes) != 1 || keyOf(deletes[0].Body) != "CONNS#b1/conn-gone" {
		t.Errorf("closed connection not forgotten: %v", deletes)
	}
}

func TestBroadcastGoesOutInParallelAndOneFailureDoesNotStopTheRest(t *testing.T) {
	f := setup(t)
	f.gateway.fail = map[string]bool{"conn-member": true}
	f.gateway.block = make(chan struct{})
	f.gateway.arrived = make(chan string, 4)

	done := make(chan struct{})
	go func() {
		f.handler.Broadcast(context.Background(), "b1", changed)
		close(done)
	}()
	// All three pushes are in flight before any returns.
	for range 3 {
		select {
		case <-f.gateway.arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("pushes are not made in parallel")
		}
	}
	close(f.gateway.block)
	<-done
	if len(f.gateway.sent["conn-peer"]) != 1 {
		t.Errorf("peer received %v after another push failed", f.gateway.sent)
	}
}

func TestBoardLevelMessagesShareTheSameDelivery(t *testing.T) {
	f := setup(t)
	f.handler.Broadcast(context.Background(), "b1", broadcast.Message{Type: broadcast.BoardDeleted})
	if sent := f.gateway.sent["conn-peer"]; len(sent) != 1 || !sameJSON(sent[0], `{"type":"deleted"}`) {
		t.Errorf("peer received %v", sent)
	}
}

func handle(f *fixture, route, connection string, headers map[string]string, board string) (int, error) {
	res, err := f.handler.Handle(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Headers:               headers,
		QueryStringParameters: map[string]string{"boardId": board},
		RequestContext:        events.APIGatewayWebsocketProxyRequestContext{RouteKey: route, ConnectionID: connection},
	})
	return res.StatusCode, err
}

func TestPingOnlyKeepsTheConnectionOpen(t *testing.T) {
	f := setup(t)
	if code, err := handle(f, "$default", "conn-member", nil, ""); err != nil || code != 200 {
		t.Errorf("ping answered %d %v", code, err)
	}
	if calls := f.db.Calls(); len(calls) != 0 {
		t.Errorf("ping reached the table: %v", calls)
	}
}

func TestDisconnectForgetsTheConnection(t *testing.T) {
	f := setup(t)
	handle(f, "$disconnect", "conn-member", nil, "")
	var forgotten []string
	for _, call := range f.db.Calls("DeleteItem") {
		forgotten = append(forgotten, keyOf(call.Body))
	}
	slices.Sort(forgotten)
	if want := []string{"CONN#conn-member/BOARD", "CONNS#b1/conn-member"}; !slices.Equal(forgotten, want) {
		t.Errorf("forgot %v, want %v", forgotten, want)
	}

	f = setup(t)
	handle(f, "$disconnect", "conn-orphan", nil, "")
	if deletes := f.db.Calls("DeleteItem"); len(deletes) != 0 {
		t.Errorf("unknown connection deleted %v", deletes)
	}
}

func (f *fixture) token(t *testing.T, sub string, key *rsa.PrivateKey) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":       testIssuer,
		"aud":       testClientID,
		"exp":       time.Now().Add(time.Hour).Unix(),
		"token_use": "id",
		"sub":       sub,
		"email":     sub + "@example.com",
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestConnectLetsOnlyMembersOpenABoardsSocket(t *testing.T) {
	f := setup(t)
	forger, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, cookie, board string
		want                int
	}{
		{"no cookie", "", "b1", 401},
		{"forged token", "id_token=" + f.token(t, "u-member", forger), "b1", 401},
		{"outsider", "id_token=" + f.token(t, "u-outsider", f.key), "b1", 403},
		{"missing board", "id_token=" + f.token(t, "u-member", f.key), "missing", 404},
		{"member", "id_token=" + f.token(t, "u-member", f.key), "b1", 200},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, _ := handle(f, "$connect", "conn-new", map[string]string{"cookie": c.cookie}, c.board)
			if code != c.want {
				t.Fatalf("got %d, want %d", code, c.want)
			}
		})
	}

	var stored []string
	for _, call := range f.db.Calls("PutItem") {
		item := call.Body["Item"].(map[string]any)
		stored = append(stored, fmt.Sprint(item["PK"].(map[string]any)["S"], "/", item["SK"].(map[string]any)["S"]))
	}
	slices.Sort(stored)
	if want := []string{"CONN#conn-new/BOARD", "CONNS#b1/conn-new"}; !slices.Equal(stored, want) {
		t.Errorf("stored %v for the member's connection, want %v", stored, want)
	}
}
