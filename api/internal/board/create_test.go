package board

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

const clientID = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"

func create(t *testing.T, body string, respond awsfake.Respond) (*awsfake.Server, *httptest.ResponseRecorder) {
	fake := awsfake.New(t, respond)
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, broadcast.Discard{})
	req := httptest.NewRequest("POST", "/boards", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithUser(req.Context(), auth.User{ID: "u1", Email: "owner@example.com"}))
	rec := httptest.NewRecorder()
	h.create(rec, req)
	return fake, rec
}

func ok(string, map[string]any) (int, any) { return 200, map[string]any{} }

func TestCreateKeepsTheClientsID(t *testing.T) {
	fake, rec := create(t, `{"id":"`+clientID+`","name":"Plans"}`, ok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %q, want 201", rec.Code, rec.Body.String())
	}
	var board Board
	json.NewDecoder(rec.Body).Decode(&board)
	if board.ID != clientID {
		t.Errorf("created board %q, want %q", board.ID, clientID)
	}

	items := fake.Calls("TransactWriteItems")[0].Body["TransactItems"].([]any)
	put := items[0].(map[string]any)["Put"].(map[string]any)
	if pk := put["Item"].(map[string]any)["PK"].(map[string]any)["S"]; pk != clientID {
		t.Errorf("stored PK %v, want %q", pk, clientID)
	}
	if put["ConditionExpression"] != "attribute_not_exists(PK)" {
		t.Errorf("board put not guarded against an existing id: %v", put["ConditionExpression"])
	}
}

func TestCreateRejectsAMalformedID(t *testing.T) {
	fake, rec := create(t, `{"id":"META","name":"Plans"}`, ok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("malformed id still wrote: %v", calls)
	}
}

func TestCreateRefusesATakenID(t *testing.T) {
	_, rec := create(t, `{"id":"`+clientID+`","name":"Plans"}`, func(op string, _ map[string]any) (int, any) {
		return 400, map[string]any{
			"__type":              "TransactionCanceledException",
			"Message":             "Transaction cancelled",
			"CancellationReasons": []any{map[string]any{"Code": "ConditionalCheckFailed"}, map[string]any{"Code": "None"}},
		}
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d %q, want 409", rec.Code, rec.Body.String())
	}
}
