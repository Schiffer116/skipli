package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Schiffer116/skipli/api/internal/auth"
)

const (
	testIssuer   = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	testClientID = "test-client"
)

// fakeAWS stands in for DynamoDB and Cognito. GetItem reads from items, a
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
		Key           attributeKey
		TransactItems []struct {
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
		fmt.Fprint(w, `{"Items":[],"Count":0}`)
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

	router := newServer(aws.Config{}, db, idp, tokens, "us-east-1_test", testClientID, "Skipli").Router

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
		{"GET", "/api/boards/{board}/members", "", 200, 403},
		{"PUT", "/api/boards/{board}", `{"name":"Renamed","description":""}`, 204, 403},
		{"PUT", "/api/boards/{board}/favorite", `{"favorite":true}`, 204, 403},
		{"POST", "/api/boards/{board}/invite", `{"email":"invitee@example.com"}`, 204, 403},
		{"DELETE", "/api/boards/{board}", "", 204, 403},
		{"POST", "/api/boards/{board}/cleanup", "", 204, 403},

		{"GET", "/api/boards/{board}/cards", "", 200, 403},
		{"POST", "/api/boards/{board}/cards", `{"name":"Card"}`, 201, 403},
		{"GET", "/api/boards/{board}/cards/c1", "", 200, 403},
		{"PUT", "/api/boards/{board}/cards/c1", `{"name":"Card","description":""}`, 200, 403},
		{"PATCH", "/api/boards/{board}/cards/c1", `{"beforeId":"c2"}`, 200, 403},
		{"PUT", "/api/boards/{board}/cards/order", `{"cardIds":["c2","c1"]}`, 204, 403},
		{"DELETE", "/api/boards/{board}/cards/c1", "", 204, 403},

		{"GET", "/api/boards/{board}/cards/c1/tasks", "", 200, 403},
		{"POST", "/api/boards/{board}/cards/c1/tasks", `{"name":"Task"}`, 201, 403},
		{"GET", "/api/boards/{board}/cards/c1/tasks/t1", "", 200, 403},
		{"PUT", "/api/boards/{board}/cards/c1/tasks/t1", `{"name":"Task","description":"","status":"done"}`, 200, 403},
		{"PATCH", "/api/boards/{board}/cards/c1/tasks/t1", `{"newCardId":"c1"}`, 200, 403},
		{"DELETE", "/api/boards/{board}/cards/c1/tasks/t1", "", 204, 403},
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
				req := httptest.NewRequest(r.method, path, strings.NewReader(r.body))
				if c.token != "" {
					req.AddCookie(&http.Cookie{Name: "id_token", Value: c.token})
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				calls := fake.takeCalls()

				want := c.want(r.member, r.outsider)
				if rec.Code != want {
					t.Fatalf("status = %d, want %d (body %q)", rec.Code, want, strings.TrimSpace(rec.Body.String()))
				}

				// Turned away before the handler ran: only the membership
				// lookups may have reached the table.
				if want == 401 && len(calls) > 0 {
					t.Errorf("unauthenticated request reached AWS: %v", calls)
				}
				if scoped && want != r.member {
					for _, call := range calls {
						if call != "GetItem" {
							t.Errorf("rejected request made a %s call: %v", call, calls)
						}
					}
				}
			})
		}
	}
}
