package task

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

const clientID = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"

// create posts a task to card c1 on board b1; transact answers the
// TransactWriteItems that guards the card and the new task's id.
func create(t *testing.T, body string, transact func() (int, any)) (*awsfake.Server, *httptest.ResponseRecorder) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		if op == "TransactWriteItems" {
			return transact()
		}
		return 200, map[string]any{"Items": []any{}}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil)
	req := httptest.NewRequest("POST", "/boards/b1/cards/c1/tasks", strings.NewReader(body))
	req.SetPathValue("boardId", "b1")
	req.SetPathValue("cardId", "c1")
	rec := httptest.NewRecorder()
	h.create(rec, req)
	return fake, rec
}

func canceled(reasons ...string) func() (int, any) {
	return func() (int, any) {
		codes := []any{}
		for _, reason := range reasons {
			codes = append(codes, map[string]any{"Code": reason})
		}
		return 400, map[string]any{
			"__type":              "TransactionCanceledException",
			"Message":             "Transaction cancelled",
			"CancellationReasons": codes,
		}
	}
}

func TestCreateKeepsTheClientsIDAndChecksTheCard(t *testing.T) {
	fake, rec := create(t, `{"id":"`+clientID+`","name":"Buy milk"}`, func() (int, any) { return 200, map[string]any{} })
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %q, want 201", rec.Code, rec.Body.String())
	}
	var task Response
	json.NewDecoder(rec.Body).Decode(&task)
	if task.ID != clientID {
		t.Errorf("created task %q, want %q", task.ID, clientID)
	}

	items := fake.Calls("TransactWriteItems")[0].Body["TransactItems"].([]any)
	check := items[0].(map[string]any)["ConditionCheck"].(map[string]any)
	if sk := check["Key"].(map[string]any)["SK"].(map[string]any)["S"]; sk != "CARD#c1" {
		t.Errorf("checked %v, want the card CARD#c1", sk)
	}
	put := items[1].(map[string]any)["Put"].(map[string]any)
	if sk := put["Item"].(map[string]any)["SK"].(map[string]any)["S"]; sk != "TASK#c1#"+clientID {
		t.Errorf("stored SK %v", sk)
	}
}

func TestCreateOnADeletedCardIsNotFound(t *testing.T) {
	_, rec := create(t, `{"id":"`+clientID+`","name":"Buy milk"}`, canceled("ConditionalCheckFailed", "None"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %q, want 404", rec.Code, rec.Body.String())
	}
}

func TestCreateRefusesATakenID(t *testing.T) {
	_, rec := create(t, `{"id":"`+clientID+`","name":"Buy milk"}`, canceled("None", "ConditionalCheckFailed"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d %q, want 409", rec.Code, rec.Body.String())
	}
}

func TestCreateRejectsAMalformedID(t *testing.T) {
	fake, rec := create(t, `{"id":"TASK#x","name":"Buy milk"}`, func() (int, any) { return 200, map[string]any{} })
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if calls := fake.Calls("TransactWriteItems"); len(calls) != 0 {
		t.Errorf("malformed id still wrote: %v", calls)
	}
}
