package task

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

var conditionFailed = map[string]any{"__type": "ConditionalCheckFailedException", "message": "missing"}

func TestUpdateOfADeletedTaskIsNotFound(t *testing.T) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		return 400, conditionFailed
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil)
	req := httptest.NewRequest("PUT", "/boards/b1/cards/c1/tasks/t1", strings.NewReader(`{"name":"Ship","status":"done"}`))
	req.SetPathValue("boardId", "b1")
	req.SetPathValue("cardId", "c1")
	req.SetPathValue("taskId", "t1")
	rec := httptest.NewRecorder()
	h.update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %q, want 404", rec.Code, rec.Body.String())
	}
	update := fake.Calls("UpdateItem")[0].Body
	if update["ConditionExpression"] != "attribute_exists(SK)" {
		t.Errorf("task update not guarded against a missing task: %v", update["ConditionExpression"])
	}
}

// move sends task t1 from card c1 to card c2; transact answers the
// TransactWriteItems that moves it.
func move(t *testing.T, transact func() (int, any)) *httptest.ResponseRecorder {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		switch op {
		case "GetItem":
			return 200, map[string]any{"Item": map[string]any{
				"ID": map[string]any{"S": "t1"}, "Name": map[string]any{"S": "Ship"},
			}}
		case "TransactWriteItems":
			return transact()
		}
		return 200, map[string]any{}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil)
	req := httptest.NewRequest("PATCH", "/boards/b1/cards/c1/tasks/t1", strings.NewReader(`{"newCardId":"c2"}`))
	req.SetPathValue("boardId", "b1")
	req.SetPathValue("cardId", "c1")
	req.SetPathValue("taskId", "t1")
	rec := httptest.NewRecorder()
	h.move(rec, req)
	return rec
}

func TestMoveIntoADeletedCardIsNotFound(t *testing.T) {
	rec := move(t, canceled("None", "ConditionalCheckFailed", "None"))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "card") {
		t.Fatalf("got %d %q, want 404 card not found", rec.Code, rec.Body.String())
	}
}

func TestMoveOfADeletedTaskIsNotFound(t *testing.T) {
	rec := move(t, canceled("ConditionalCheckFailed", "None", "None"))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "task") {
		t.Fatalf("got %d %q, want 404 task not found", rec.Code, rec.Body.String())
	}
}
