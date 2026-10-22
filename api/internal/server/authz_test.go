package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

const (
	testIssuer   = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	testClientID = "test-client"
)

// fakeAWS stands in for DynamoDB and Cognito. GetItem reads from items, a
// Query on a board's partition gives back its items in key order, a
// transaction's attribute_exists conditions are checked against them, and
// every other call succeeds with an empty result.
type fakeAWS struct {
	items map[string]string // "PK/SK" -> DynamoDB JSON item

	mu    sync.Mutex
	calls []string
}

type attributeKey map[string]map[string]string

func (k attributeKey) String() string {
	return k["PK"]["S"] + "/" + k["SK"]["S"]
}

func (f *fakeAWS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, op, _ := strings.Cut(r.Header.Get("X-Amz-Target"), ".")
	f.mu.Lock()
	f.calls = append(f.calls, op)
	f.mu.Unlock()

	var body struct {
		Key                       attributeKey
		KeyConditionExpression    string
		ExpressionAttributeValues map[string]map[string]string
		TransactItems             []struct {
			Update *struct {
				Key                 attributeKey
				ConditionExpression string
			}
		}
	}
	json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")

	switch op {
	case "GetItem":
		if item, ok := f.items[body.Key.String()]; ok {
			fmt.Fprintf(w, `{"Item":%s}`, item)
			return
		}
		fmt.Fprint(w, `{}`)
	case "Query":
		// Only whole-partition reads (a board read) find anything; other
		// queries, by index or key prefix, come back empty.
		var found []string
		if !strings.Contains(body.KeyConditionExpression, "begins_with") {
			keys := slices.Sorted(maps.Keys(f.items))
			for _, v := range body.ExpressionAttributeValues {
				for _, k := range keys {
					if strings.HasPrefix(k, v["S"]+"/") {
						found = append(found, f.items[k])
					}
				}
			}
		}
		fmt.Fprintf(w, `{"Items":[%s],"Count":%d}`, strings.Join(found, ","), len(found))
	case "TransactWriteItems":
		var reasons []string
		failed := false
		for _, item := range body.TransactItems {
			code := "None"
			if u := item.Update; u != nil && strings.Contains(u.ConditionExpression, "attribute_exists") {
				if _, ok := f.items[u.Key.String()]; !ok {
					code = "ConditionalCheckFailed"
					failed = true
				}
			}
			reasons = append(reasons, fmt.Sprintf(`{"Code":%q}`, code))
		}
		if failed {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"__type":"com.amazonaws.dynamodb.v20120810#TransactionCanceledException","message":"canceled","CancellationReasons":[%s]}`, strings.Join(reasons, ","))
			return
		}
		fmt.Fprint(w, `{}`)
	case "AdminCreateUser":
		fmt.Fprint(w, `{"User":{"Username":"invitee","Attributes":[{"Name":"sub","Value":"u-invitee"},{"Name":"email","Value":"invitee@example.com"}]}}`)
	default:
		fmt.Fprint(w, `{}`)
	}
}

// recordedBroadcasts remembers what the handlers broadcast, by board.
type recordedBroadcasts struct {
	mu   sync.Mutex
	sent []string
}

func (r *recordedBroadcasts) Broadcast(_ context.Context, boardID string, m broadcast.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, boardID+" "+m.Type)
}

func (r *recordedBroadcasts) Present(context.Context, string) []broadcast.Person {
	return []broadcast.Person{{ID: "u-member", Name: "Member", Email: "member@example.com"}}
}

func (r *recordedBroadcasts) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	sent := r.sent
	r.sent = nil
	return sent
}

func (f *fakeAWS) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

func signToken(t *testing.T, key *rsa.PrivateKey, sub string) string {
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

func TestBoardRoutesRequireMembership(t *testing.T) {
	fake := &fakeAWS{items: map[string]string{
		"b1/META":            `{"PK":{"S":"b1"},"SK":{"S":"META"},"ID":{"S":"b1"},"Owner":{"S":"u-member"},"Name":{"S":"Board"},"Description":{"S":""}}`,
		"b1/MEMBER#u-member": `{"PK":{"S":"b1"},"SK":{"S":"MEMBER#u-member"},"Member":{"S":"u-member"},"Email":{"S":"u-member@example.com"}}`,
		"b1/CARD#c1":         `{"PK":{"S":"b1"},"SK":{"S":"CARD#c1"},"ID":{"S":"c1"},"Name":{"S":"Card 1"},"Description":{"S":""},"Order":{"N":"1000"}}`,
		"b1/CARD#c2":         `{"PK":{"S":"b1"},"SK":{"S":"CARD#c2"},"ID":{"S":"c2"},"Name":{"S":"Card 2"},"Description":{"S":""},"Order":{"N":"2000"}}`,
		"b1/TASK#c1#t1":      `{"PK":{"S":"b1"},"SK":{"S":"TASK#c1#t1"},"ID":{"S":"t1"},"CardID":{"S":"c1"},"Name":{"S":"Task"},"Description":{"S":""},"Status":{"S":"pending"},"Order":{"N":"1000"}}`,
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	db := dynamodb.New(dynamodb.Options{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String(srv.URL),
		Credentials:      aws.AnonymousCredentials{},
		RetryMaxAttempts: 1,
	})
	idp := cognitoidentityprovider.New(cognitoidentityprovider.Options{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String(srv.URL),
		Credentials:      aws.AnonymousCredentials{},
		RetryMaxAttempts: 1,
	})

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenVerifierWithKeyfunc(func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, testIssuer, testClientID)

	canReceiveMail := func(context.Context, string) bool { return true }
	broadcasts := &recordedBroadcasts{}
	router := newRouter(db, idp, tokens, "us-east-1_test", testClientID, "Skipli", canReceiveMail, broadcasts, broadcasts)

	// {board} is replaced with the board under test; routes without it
	// aren't scoped to a board and only need a signed-in caller.
	routes := []struct {
		method, path, body string
		member             int // status for a member of the board
		outsider           int // status for a signed-in user who isn't
	}{
		{"GET", "/api/boards", "", 200, 200},
		{"POST", "/api/boards", `{"name":"New"}`, 201, 201},
		{"PUT", "/api/boards/order", `{"boardIds":["b1"]}`, 204, 403},
		{"GET", "/api/boards/{board}", "", 200, 403},
		{"GET", "/api/boards/{board}/presence", "", 200, 403},
		{"PUT", "/api/boards/{board}", `{"name":"Renamed","description":""}`, 204, 403},
		{"PUT", "/api/boards/{board}/favorite", `{"favorite":true}`, 204, 403},
		{"POST", "/api/boards/{board}/invite", `{"email":"invitee@example.com"}`, 204, 403},
		{"DELETE", "/api/boards/{board}", "", 204, 403},
	}

	// Every kind of Board change is taken from members only.
	for _, change := range []string{
		`{"type":"createCard","boardId":"{board}","cardId":"0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69","name":"Card","createdAt":""}`,
		`{"type":"updateCard","boardId":"{board}","cardId":"c1","name":"Card","description":""}`,
		`{"type":"deleteCard","boardId":"{board}","cardId":"c2"}`,
		`{"type":"moveCard","boardId":"{board}","cardId":"c1","beforeId":"c2","afterId":null}`,
		`{"type":"createTask","boardId":"{board}","cardId":"c1","taskId":"0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69","name":"Task"}`,
		`{"type":"updateTask","boardId":"{board}","cardId":"c1","taskId":"t1","name":"Task","description":"","status":"done"}`,
		`{"type":"deleteTask","boardId":"{board}","cardId":"c1","taskId":"t1"}`,
		`{"type":"moveTask","boardId":"{board}","cardId":"c1","taskId":"t1","newCardId":"c2","beforeId":null,"afterId":null}`,
		`{"type":"cleanUp","boardId":"{board}","taskIds":[]}`,
	} {
		routes = append(routes, struct {
			method, path, body string
			member             int
			outsider           int
		}{"POST", "/api/boards/{board}/changes", change, 204, 403})
	}

	callers := []struct {
		name  string
		token string
		board string
		want  func(member, outsider int) int
	}{
		{"no cookie", "", "b1", func(int, int) int { return 401 }},
		{"forged token", signToken(t, otherKey, "u-member"), "b1", func(int, int) int { return 401 }},
		{"outsider", signToken(t, key, "u-outsider"), "b1", func(_, outsider int) int { return outsider }},
		{"member", signToken(t, key, "u-member"), "b1", func(member, _ int) int { return member }},
		{"missing board", signToken(t, key, "u-member"), "missing", func(member, _ int) int { return 404 }},
	}

	for _, r := range routes {
		scoped := strings.Contains(r.path, "{board}")
		for _, c := range callers {
			if c.board != "b1" && !scoped {
				continue
			}
			path := strings.ReplaceAll(r.path, "{board}", c.board)
			t.Run(fmt.Sprintf("%s %s as %s", r.method, path, c.name), func(t *testing.T) {
				broadcasts.take()
				body := strings.ReplaceAll(r.body, "{board}", c.board)
				req := httptest.NewRequest(r.method, path, strings.NewReader(body))
				if c.token != "" {
					req.AddCookie(&http.Cookie{Name: "id_token", Value: c.token})
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				calls := fake.takeCalls()
				sent := broadcasts.take()

				want := c.want(r.member, r.outsider)
				if rec.Code != want {
					t.Fatalf("status = %d, want %d (body %q)", rec.Code, want, strings.TrimSpace(rec.Body.String()))
				}

				if want == 204 && (r.method != "GET") && strings.HasSuffix(path, "/changes") && len(sent) != 1 {
					t.Errorf("accepted change broadcast %v, want one", sent)
				}

				// Turned away before the handler ran: only the membership
				// lookups may have reached the table.
				if want == 401 && len(calls) > 0 {
					t.Errorf("unauthenticated request reached AWS: %v", calls)
				}
				if scoped && want != r.member {
					if len(sent) != 0 {
						t.Errorf("rejected request was broadcast: %v", sent)
					}
					for _, call := range calls {
						if call != "GetItem" {
							t.Errorf("rejected request made a %s call: %v", call, calls)
						}
					}
				}
			})
		}
	}

	// A member opening the board gets all of it from one membership check,
	// one Query and one profile lookup.
	t.Run("whole board", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/boards/b1", nil)
		req.AddCookie(&http.Cookie{Name: "id_token", Value: signToken(t, key, "u-member")})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		calls := fake.takeCalls()

		want := `{"id":"b1","owner":"u-member","name":"Board","description":"",` +
			`"members":[{"id":"u-member","email":"u-member@example.com","name":"u-member"}],` +
			`"cards":[{"id":"c1","name":"Card 1","description":"","tasks":[{"id":"t1","cardId":"c1","name":"Task","description":"","status":"pending"}]},` +
			`{"id":"c2","name":"Card 2","description":"","tasks":[]}]}`
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != 200 || got != want {
			t.Errorf("got %d %s\nwant 200 %s", rec.Code, got, want)
		}
		if got := strings.Join(calls, ","); got != "GetItem,Query,BatchGetItem" {
			t.Errorf("calls = %s, want GetItem,Query,BatchGetItem", got)
		}
	})
}
