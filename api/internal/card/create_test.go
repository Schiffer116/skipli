package card

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

const clientID = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"

func create(t *testing.T, body string, put func() (int, any)) (*awsfake.Server, *httptest.ResponseRecorder) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		if op == "PutItem" {
			return put()
		}
		return 200, map[string]any{"Items": []any{}}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil)
	req := httptest.NewRequest("POST", "/boards/b1/cards", strings.NewReader(body))
	req.SetPathValue("boardId", "b1")
	rec := httptest.NewRecorder()
	h.create(rec, req)
	return fake, rec
}

func TestCreateKeepsTheClientsID(t *testing.T) {
	fake, rec := create(t, `{"id":"`+clientID+`","name":"Ideas"}`, func() (int, any) { return 200, map[string]any{} })
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %q, want 201", rec.Code, rec.Body.String())
	}
	var card Response
	json.NewDecoder(rec.Body).Decode(&card)
	if card.ID != clientID {
		t.Errorf("created card %q, want %q", card.ID, clientID)
	}
	put := fake.Calls("PutItem")[0].Body
	if put["ConditionExpression"] != "attribute_not_exists(SK)" {
		t.Errorf("card put not guarded against an existing id: %v", put["ConditionExpression"])
	}
}

func TestCreateRefusesATakenID(t *testing.T) {
	_, rec := create(t, `{"id":"`+clientID+`","name":"Ideas"}`, func() (int, any) {
		return 400, map[string]any{"__type": "ConditionalCheckFailedException", "message": "exists"}
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d %q, want 409", rec.Code, rec.Body.String())
	}
}
