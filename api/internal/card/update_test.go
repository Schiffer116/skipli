package card

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

func TestUpdateOfADeletedCardIsNotFound(t *testing.T) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		return 400, map[string]any{"__type": "ConditionalCheckFailedException", "message": "missing"}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil)
	req := httptest.NewRequest("PUT", "/boards/b1/cards/c1", strings.NewReader(`{"name":"Ideas"}`))
	req.SetPathValue("boardId", "b1")
	req.SetPathValue("cardId", "c1")
	rec := httptest.NewRecorder()
	h.update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %q, want 404", rec.Code, rec.Body.String())
	}
	update := fake.Calls("UpdateItem")[0].Body
	if update["ConditionExpression"] != "attribute_exists(SK)" {
		t.Errorf("card update not guarded against a missing card: %v", update["ConditionExpression"])
	}
}
