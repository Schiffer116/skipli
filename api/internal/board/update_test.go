package board

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

func TestRenameOfADeletedBoardIsNotFound(t *testing.T) {
	fake := awsfake.New(t, func(string, map[string]any) (int, any) {
		return 400, map[string]any{"__type": "ConditionalCheckFailedException", "message": "missing"}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil)
	req := httptest.NewRequest("PUT", "/boards/b1", strings.NewReader(`{"name":"Plans"}`))
	req.SetPathValue("id", "b1")
	rec := httptest.NewRecorder()
	h.put(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %q, want 404", rec.Code, rec.Body.String())
	}
	update := fake.Calls("UpdateItem")[0].Body
	if update["ConditionExpression"] != "attribute_exists(SK)" {
		t.Errorf("board rename not guarded against a missing board: %v", update["ConditionExpression"])
	}
}
