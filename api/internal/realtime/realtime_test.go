package realtime

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
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
)

const (
	testIssuer   = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	testClientID = "test-client"
	newID        = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"
)

// The table: board b1 with its member, two cards and a task, and the
// connections open on it. conn-removed belongs to someone no longer a member.
var table = map[string]map[string]any{
	"b1/META":              {},
	"b1/MEMBER#u-member":   {},
	"b1/CARD#c1":           {"Order": map[string]any{"N": "1000"}},
	"b1/CARD#c2":           {"Order": map[string]any{"N": "2000"}},
	"b1/TASK#c1#t1":        {"Name": map[string]any{"S": "Ship"}, "Order": map[string]any{"N": "1000"}},
	"b1/CONN#conn-member":  {"User": map[string]any{"S": "u-member"}},
	"b1/CONN#conn-peer":    {"User": map[string]any{"S": "u-member"}},
	"b1/CONN#conn-gone":    {"User": map[string]any{"S": "u-member"}},
	"b1/CONN#conn-removed": {"User": map[string]any{"S": "u-removed"}},
	"b2/META":              {},
	"b2/CONN#conn-other":   {"User": map[string]any{"S": "u-member"}},
}

func keyOf(body map[string]any) string {
	key, _ := body["Key"].(map[string]any)
	pk, _ := key["PK"].(map[string]any)
	sk, _ := key["SK"].(map[string]any)
	return fmt.Sprint(pk["S"], "/", sk["S"])
}

// fakeTable answers reads from table; every write succeeds, except that an
// update of something not in the table fails its condition as DynamoDB would.
func fakeTable(t *testing.T) *awsfake.Server {
	return awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "GetItem":
			if item, ok := table[keyOf(body)]; ok {
				return 200, map[string]any{"Item": item}
			}
			return 200, map[string]any{}
		case "UpdateItem":
			if _, ok := table[keyOf(body)]; !ok {
				return 400, map[string]any{"__type": "ConditionalCheckFailedException", "message": "missing"}
			}
		case "Query":
			values, _ := json.Marshal(body["ExpressionAttributeValues"])
			if !strings.Contains(string(values), "CONN#") {
				return 200, map[string]any{"Items": []any{}}
			}
			var items []any
			for key := range table {
				pk, sk, _ := strings.Cut(key, "/")
				if strings.Contains(string(values), `"`+pk+`"`) && strings.HasPrefix(sk, "CONN#") {
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
}

func (g *fakeGateway) PostToConnection(_ context.Context, in *apigatewaymanagementapi.PostToConnectionInput, _ ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error) {
	if *in.ConnectionId == "conn-gone" {
		return nil, &gwtypes.GoneException{}
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
	f.handler = newHandler(f.db.DynamoDB(), "Skipli", tokens, func(endpoint string) Gateway {
		if endpoint != "https://abc.execute-api.us-east-1.amazonaws.com/ws" {
			t.Errorf("gateway endpoint %q", endpoint)
		}
		return f.gateway
	})
	return f
}

// send delivers a message from a connection and returns the ack it got.
func (f *fixture) send(t *testing.T, connection, body string) ack {
	t.Helper()
	res, err := f.handler.Handle(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: body,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			RouteKey:     "$default",
			ConnectionID: connection,
			DomainName:   "abc.execute-api.us-east-1.amazonaws.com",
			Stage:        "ws",
		},
	})
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("got %d %v, want 200", res.StatusCode, err)
	}
	var a ack
	if res.Body != "" {
		if err := json.Unmarshal([]byte(res.Body), &a); err != nil {
			t.Fatalf("reply %q: %v", res.Body, err)
		}
	}
	return a
}

func changeMessage(id, change string) string {
	return `{"action":"change","id":"` + id + `","change":` + change + `}`
}

func TestAcceptedChangeIsAckedAndBroadcastToTheBoardsOtherConnections(t *testing.T) {
	f := setup(t)
	change := `{"type":"updateTask","boardId":"b1","cardId":"c1","taskId":"t1","name":"Ship it","description":"","status":"done"}`

	got := f.send(t, "conn-member", changeMessage("m1", change))
	if got != (ack{Type: "ack", ID: "m1", Status: 200}) {
		t.Fatalf("ack = %+v", got)
	}
	if len(f.db.Calls("UpdateItem")) != 1 {
		t.Errorf("change not applied: %v", f.db.Calls())
	}

	want := `{"change":` + change + `,"type":"change"}`
	for _, conn := range []string{"conn-peer", "conn-removed"} {
		if sent := f.gateway.sent[conn]; len(sent) != 1 || !sameJSON(sent[0], want) {
			t.Errorf("%s received %v, want %s", conn, sent, want)
		}
	}
	for _, conn := range []string{"conn-member", "conn-other"} {
		if sent := f.gateway.sent[conn]; len(sent) != 0 {
			t.Errorf("%s received %v, want nothing", conn, sent)
		}
	}
	deletes := f.db.Calls("DeleteItem")
	if len(deletes) != 1 || keyOf(deletes[0].Body) != "b1/CONN#conn-gone" {
		t.Errorf("closed connection not forgotten: %v", deletes)
	}
}

func sameJSON(a, b string) bool {
	var x, y any
	json.Unmarshal([]byte(a), &x)
	json.Unmarshal([]byte(b), &y)
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

func TestRefusedChangeIsAckedWithItsStatusAndNotBroadcast(t *testing.T) {
	f := setup(t)
	got := f.send(t, "conn-member", changeMessage("m2",
		`{"type":"updateCard","boardId":"b1","cardId":"deleted","name":"Ideas","description":""}`))
	if got.ID != "m2" || got.Status != 404 || got.Error != "card not found" {
		t.Fatalf("ack = %+v, want 404 card not found", got)
	}
	if len(f.gateway.sent) != 0 {
		t.Errorf("refused change was broadcast: %v", f.gateway.sent)
	}
}

func TestMalformedMessagesAreRefused(t *testing.T) {
	f := setup(t)
	for _, body := range []string{
		`not json`,
		`{"boardId":"b1","event":"change","args":[]}`,
		changeMessage("m3", `{"type":"createTask","boardId":"b1","cardId":"c1","taskId":"TASK#x","name":"x"}`),
	} {
		if got := f.send(t, "conn-member", body); got.Status != 400 {
			t.Errorf("%s: ack = %+v, want 400", body, got)
		}
	}
	if writes := f.db.Calls("PutItem", "UpdateItem", "TransactWriteItems", "BatchWriteItem"); len(writes) != 0 {
		t.Errorf("malformed message wrote: %v", writes)
	}
}

func TestPingOnlyKeepsTheConnectionOpen(t *testing.T) {
	f := setup(t)
	if got := f.send(t, "conn-member", `{"action":"ping"}`); got != (ack{}) {
		t.Errorf("ping answered %+v", got)
	}
	if calls := f.db.Calls(); len(calls) != 0 {
		t.Errorf("ping reached the table: %v", calls)
	}
}

// Every kind of Board change is only applied for a member, on a connection
// opened on the board it changes.
func TestEveryChangeTypeRequiresAMembersConnectionOnTheBoard(t *testing.T) {
	changes := []string{
		`{"type":"createCard","boardId":"{board}","cardId":"` + newID + `","name":"Card","createdAt":""}`,
		`{"type":"updateCard","boardId":"{board}","cardId":"c1","name":"Card","description":""}`,
		`{"type":"deleteCard","boardId":"{board}","cardId":"c2"}`,
		`{"type":"moveCard","boardId":"{board}","cardId":"c1","beforeId":"c2","afterId":null}`,
		`{"type":"createTask","boardId":"{board}","cardId":"c1","taskId":"` + newID + `","name":"Task"}`,
		`{"type":"updateTask","boardId":"{board}","cardId":"c1","taskId":"t1","name":"Task","description":"","status":"done"}`,
		`{"type":"deleteTask","boardId":"{board}","cardId":"c1","taskId":"t1"}`,
		`{"type":"moveTask","boardId":"{board}","cardId":"c1","taskId":"t1","newCardId":"c2","beforeId":null,"afterId":null}`,
		`{"type":"cleanUp","boardId":"{board}"}`,
	}
	callers := []struct {
		name, connection, board string
		want                    int
	}{
		{"member", "conn-member", "b1", 200},
		{"removed member", "conn-removed", "b1", 403},
		{"connection on another board", "conn-other", "b1", 403},
		{"unknown connection", "conn-unknown", "b1", 403},
		{"missing board", "conn-member", "missing", 404},
	}

	for _, change := range changes {
		for _, c := range callers {
			kind := strings.Split(change, `"`)[3]
			t.Run(kind+" as "+c.name, func(t *testing.T) {
				f := setup(t)
				got := f.send(t, c.connection, changeMessage("m", strings.ReplaceAll(change, "{board}", c.board)))
				if got.Status != c.want {
					t.Fatalf("ack = %+v, want %d", got, c.want)
				}
				if c.want == 200 {
					if len(f.gateway.sent["conn-peer"]) != 1 {
						t.Errorf("accepted change not broadcast: %v", f.gateway.sent)
					}
					return
				}
				for _, call := range f.db.Calls() {
					if call.Op != "GetItem" {
						t.Errorf("refused change made a %s call", call.Op)
					}
				}
				if len(f.gateway.sent) != 0 {
					t.Errorf("refused change was broadcast: %v", f.gateway.sent)
				}
			})
		}
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
			res, _ := f.handler.Handle(context.Background(), events.APIGatewayWebsocketProxyRequest{
				Headers:               map[string]string{"cookie": c.cookie},
				QueryStringParameters: map[string]string{"boardId": c.board},
				RequestContext:        events.APIGatewayWebsocketProxyRequestContext{RouteKey: "$connect", ConnectionID: "conn-new"},
			})
			if res.StatusCode != c.want {
				t.Fatalf("got %d, want %d", res.StatusCode, c.want)
			}
		})
	}

	puts := f.db.Calls("PutItem")
	if len(puts) != 1 {
		t.Fatalf("stored %d connections, want the member's only", len(puts))
	}
	item := puts[0].Body["Item"].(map[string]any)
	if item["SK"].(map[string]any)["S"] != "CONN#conn-new" || item["User"].(map[string]any)["S"] != "u-member" {
		t.Errorf("stored connection %v", item)
	}
}
