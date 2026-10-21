package boardchange

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

const clientID = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"

var conditionFailed = map[string]any{"__type": "ConditionalCheckFailedException", "message": "failed"}

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

func ok() (int, any) { return 200, map[string]any{} }

// apply applies a change to board b1. write answers the table's writes;
// reads find a task t1 and nothing else.
func apply(t *testing.T, change string, write func() (int, any)) (*awsfake.Server, error) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		switch op {
		case "PutItem", "UpdateItem", "TransactWriteItems", "DeleteItem":
			return write()
		case "GetItem":
			return 200, map[string]any{"Item": map[string]any{
				"ID": map[string]any{"S": "t1"}, "Name": map[string]any{"S": "Ship"},
			}}
		}
		return 200, map[string]any{"Items": []any{}}
	})
	var c Change
	if err := json.Unmarshal([]byte(change), &c); err != nil {
		t.Fatal(err)
	}
	return fake, NewStore(fake.DynamoDB(), "Skipli").Apply(context.Background(), c)
}

func wantStatus(t *testing.T, err error, want int, mentions string) {
	t.Helper()
	if got := Status(err); err == nil && want != http.StatusOK || err != nil && got != want {
		t.Fatalf("got %v (status %d), want %d", err, got, want)
	}
	if err != nil && !strings.Contains(err.Error(), mentions) {
		t.Errorf("got %q, want it to mention %q", err, mentions)
	}
}

func TestCreateCardKeepsTheClientsID(t *testing.T) {
	fake, err := apply(t, `{"type":"createCard","boardId":"b1","cardId":"`+clientID+`","name":"Ideas"}`, ok)
	wantStatus(t, err, http.StatusOK, "")
	put := fake.Calls("PutItem")[0].Body
	if put["ConditionExpression"] != "attribute_not_exists(SK)" {
		t.Errorf("card put not guarded against an existing id: %v", put["ConditionExpression"])
	}
	if sk := put["Item"].(map[string]any)["SK"].(map[string]any)["S"]; sk != "CARD#"+clientID {
		t.Errorf("stored SK %v", sk)
	}
}

func TestCreateCardRefusesATakenID(t *testing.T) {
	_, err := apply(t, `{"type":"createCard","boardId":"b1","cardId":"`+clientID+`","name":"Ideas"}`, func() (int, any) {
		return 400, conditionFailed
	})
	wantStatus(t, err, http.StatusConflict, "exists")
}

func TestCreateTaskKeepsTheClientsIDAndChecksTheCard(t *testing.T) {
	fake, err := apply(t, `{"type":"createTask","boardId":"b1","cardId":"c1","taskId":"`+clientID+`","name":"Buy milk"}`, ok)
	wantStatus(t, err, http.StatusOK, "")

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

func TestCreateTaskOnADeletedCardIsNotFound(t *testing.T) {
	_, err := apply(t, `{"type":"createTask","boardId":"b1","cardId":"c1","taskId":"`+clientID+`","name":"Buy milk"}`,
		canceled("ConditionalCheckFailed", "None"))
	wantStatus(t, err, http.StatusNotFound, "card")
}

func TestCreateTaskRefusesATakenID(t *testing.T) {
	_, err := apply(t, `{"type":"createTask","boardId":"b1","cardId":"c1","taskId":"`+clientID+`","name":"Buy milk"}`,
		canceled("None", "ConditionalCheckFailed"))
	wantStatus(t, err, http.StatusConflict, "exists")
}

func TestMalformedChangesAreRefusedWithoutWriting(t *testing.T) {
	for _, change := range []string{
		`{"type":"createTask","boardId":"b1","cardId":"c1","taskId":"TASK#x","name":"Buy milk"}`,
		`{"type":"createCard","boardId":"b1","name":"No id"}`,
		`{"type":"updateTask","boardId":"b1","cardId":"c1","name":"No task"}`,
		`{"type":"moveTask","boardId":"b1","cardId":"c1","taskId":"t1"}`,
		`{"type":"deleteCard","cardId":"c1"}`,
		`{"type":"renameBoard","boardId":"b1"}`,
	} {
		fake, err := apply(t, change, ok)
		if Status(err) != http.StatusBadRequest {
			t.Errorf("%s: got %v, want 400", change, err)
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("%s: still reached the table: %v", change, calls)
		}
	}
}

func TestUpdatesOfDeletedItemsAreNotFound(t *testing.T) {
	for change, mentions := range map[string]string{
		`{"type":"updateCard","boardId":"b1","cardId":"c1","name":"Ideas"}`:               "card",
		`{"type":"updateTask","boardId":"b1","cardId":"c1","taskId":"t1","name":"Ship"}`:  "task",
		`{"type":"moveTask","boardId":"b1","cardId":"c1","taskId":"t1","newCardId":"c1"}`: "task",
		`{"type":"moveCard","boardId":"b1","cardId":"c1","beforeId":null,"afterId":"t1"}`: "card",
	} {
		fake, err := apply(t, change, func() (int, any) { return 400, conditionFailed })
		wantStatus(t, err, http.StatusNotFound, mentions)
		update := fake.Calls("UpdateItem")[0].Body
		if update["ConditionExpression"] != "attribute_exists(SK)" {
			t.Errorf("%s: not guarded against a missing item: %v", change, update["ConditionExpression"])
		}
	}
}

func TestMoveTaskIntoADeletedCardIsNotFound(t *testing.T) {
	_, err := apply(t, `{"type":"moveTask","boardId":"b1","cardId":"c1","taskId":"t1","newCardId":"c2"}`,
		canceled("None", "ConditionalCheckFailed", "None"))
	wantStatus(t, err, http.StatusNotFound, "card")
}

func TestMoveOfADeletedTaskIsNotFound(t *testing.T) {
	_, err := apply(t, `{"type":"moveTask","boardId":"b1","cardId":"c1","taskId":"t1","newCardId":"c2"}`,
		canceled("ConditionalCheckFailed", "None", "None"))
	wantStatus(t, err, http.StatusNotFound, "task")
}

func TestChangesAreSentOnInTheShapeTheAppUses(t *testing.T) {
	for _, change := range []string{
		`{"type":"createCard","boardId":"b1","cardId":"c1","name":"Ideas","createdAt":"2026-10-21T00:00:00Z"}`,
		`{"type":"moveCard","boardId":"b1","cardId":"c1","beforeId":null,"afterId":"c2"}`,
		`{"type":"moveTask","boardId":"b1","cardId":"c1","taskId":"t1","newCardId":"c2","beforeId":"t2","afterId":null}`,
		`{"type":"updateTask","boardId":"b1","cardId":"c1","taskId":"t1","name":"Ship","description":"","status":"done"}`,
		`{"type":"cleanUp","boardId":"b1"}`,
	} {
		var c Change
		if err := json.Unmarshal([]byte(change), &c); err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		var want, got map[string]any
		json.Unmarshal([]byte(change), &want)
		json.Unmarshal(out, &got)
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("got %s, want %s", gotJSON, wantJSON)
		}
	}
}
