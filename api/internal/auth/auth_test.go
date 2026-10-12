package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

// cooldownTable fakes the DynamoDB side of Cooldown: a conditional put that
// fails while an unexpired item exists, and a delete.
type cooldownTable struct {
	mu      sync.Mutex
	expires map[string]string
}

func (c *cooldownTable) respond(op string, body map[string]any) (int, any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch op {
	case "PutItem":
		item := body["Item"].(map[string]any)
		pk := attr(item, "PK", "S")
		now, _ := strconv.ParseInt(attr(body["ExpressionAttributeValues"].(map[string]any), ":now", "N"), 10, 64)
		if old, ok := c.expires[pk]; ok {
			if expires, _ := strconv.ParseInt(old, 10, 64); expires > now {
				return 400, map[string]any{
					"__type":  "com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException",
					"message": "The conditional request failed",
					"Item":    map[string]any{"PK": map[string]any{"S": pk}, "ExpiresAt": map[string]any{"N": old}},
				}
			}
		}
		c.expires[pk] = attr(item, "ExpiresAt", "N")
	case "DeleteItem":
		delete(c.expires, attr(body["Key"].(map[string]any), "PK", "S"))
	}
	return 200, map[string]any{}
}

func attr(item map[string]any, name, kind string) string {
	v, _ := item[name].(map[string]any)[kind].(string)
	return v
}

type sendTest struct {
	fake       *awsfake.Server
	handler    *Handler
	failSignIn atomic.Bool
}

func newSendTest(t *testing.T) *sendTest {
	st := &sendTest{}
	table := &cooldownTable{expires: map[string]string{}}
	st.fake = awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "PutItem", "DeleteItem":
			return table.respond(op, body)
		case "AdminCreateUser":
			return 200, map[string]any{"User": map[string]any{"Attributes": []any{
				map[string]any{"Name": "sub", "Value": "u1"},
				map[string]any{"Name": "email", "Value": body["Username"]},
			}}}
		case "InitiateAuth":
			if st.failSignIn.Load() {
				return 500, map[string]any{"__type": "InternalErrorException", "message": "boom"}
			}
			return 200, map[string]any{"Session": "s1"}
		}
		return 200, map[string]any{}
	})

	idp := st.fake.Cognito()
	st.handler = NewHandler(idp, "client", NewDirectory(idp, "pool"), nil, NewCooldown(st.fake.DynamoDB(), "Skipli"))
	st.handler.canReceiveMail = func(_ context.Context, email string) bool {
		return strings.HasSuffix(email, "@example.com")
	}
	return st
}

func (st *sendTest) send(email string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(SendVerificationEmailRequest{Email: email})
	rec := httptest.NewRecorder()
	st.handler.sendVerificationEmail(rec, httptest.NewRequest("POST", "/auth/send", strings.NewReader(string(body))))
	return rec
}

func TestSendNormalizesEmailAndStartsCooldown(t *testing.T) {
	st := newSendTest(t)

	rec := st.send("  Alice@Example.COM ")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %q, want 200", rec.Code, rec.Body.String())
	}

	puts := st.fake.Calls("PutItem")
	if len(puts) != 1 {
		t.Fatalf("got %d PutItem calls, want 1", len(puts))
	}
	if pk := attr(puts[0].Body["Item"].(map[string]any), "PK", "S"); pk != "EMAIL#alice@example.com" {
		t.Errorf("cooldown key = %q, want EMAIL#alice@example.com", pk)
	}
	if cond, _ := puts[0].Body["ConditionExpression"].(string); cond == "" {
		t.Error("cooldown put is not conditional")
	}

	for _, c := range st.fake.Calls("AdminCreateUser", "InitiateAuth") {
		got := c.Body["Username"]
		if c.Op == "InitiateAuth" {
			got = c.Body["AuthParameters"].(map[string]any)["USERNAME"]
		}
		if got != "alice@example.com" {
			t.Errorf("%s username = %v, want alice@example.com", c.Op, got)
		}
	}
}

func TestSendWithinCooldownIsRejected(t *testing.T) {
	st := newSendTest(t)

	if rec := st.send("alice@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("first send: got %d, want 200", rec.Code)
	}
	cognitoCalls := len(st.fake.Calls("AdminCreateUser", "InitiateAuth"))

	rec := st.send("ALICE@example.com")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second send: got %d %q, want 429", rec.Code, rec.Body.String())
	}
	retry, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || retry < 1 || retry > int(SendCooldown.Seconds()) {
		t.Errorf("Retry-After = %q, want 1..%d", rec.Header().Get("Retry-After"), int(SendCooldown.Seconds()))
	}
	if got := len(st.fake.Calls("AdminCreateUser", "InitiateAuth")); got != cognitoCalls {
		t.Errorf("rejected send reached Cognito: %d calls, want %d", got, cognitoCalls)
	}

	if rec := st.send("bob@example.com"); rec.Code != http.StatusOK {
		t.Errorf("another address: got %d, want 200", rec.Code)
	}
}

func TestSendRejectsUndeliverableEmail(t *testing.T) {
	st := newSendTest(t)

	rec := st.send("nobody@nowhere.invalid")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if calls := st.fake.Calls(); len(calls) != 0 {
		t.Errorf("undeliverable address reached AWS: %v", calls)
	}
}

func TestSendFailureCancelsCooldown(t *testing.T) {
	st := newSendTest(t)
	st.failSignIn.Store(true)

	if rec := st.send("alice@example.com"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if len(st.fake.Calls("DeleteItem")) != 1 {
		t.Fatal("failed send did not cancel its cooldown")
	}

	st.failSignIn.Store(false)
	if rec := st.send("alice@example.com"); rec.Code != http.StatusOK {
		t.Errorf("retry after failure: got %d %q, want 200", rec.Code, rec.Body.String())
	}
}

func TestCanReceiveMailRejectsMalformedAddresses(t *testing.T) {
	for _, email := range []string{"", "not-an-email", "Bob <bob@example.com>", "bob@example.com, eve@example.com"} {
		if CanReceiveMail(context.Background(), email) {
			t.Errorf("CanReceiveMail(%q) = true, want false", email)
		}
	}
}
