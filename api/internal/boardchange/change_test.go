package boardchange

import (
	"context"
	"encoding/json"
	"fmt"
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

// deleteFake serves a card holding the given task keys across pages of
// pageSize, and records how the writes arrive. unprocessed makes the first
// BatchWriteItem leave its last request undone.
func deleteFake(t *testing.T, tasks, pageSize int, unprocessed bool) *awsfake.Server {
	left := true
	return awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "Query":
			start := 0
			if key, ok := body["ExclusiveStartKey"].(map[string]any); ok {
				fmt.Sscanf(key["SK"].(map[string]any)["S"].(string), "TASK#c1#%d", &start)
				start++
			}
			end := min(start+pageSize, tasks)
			items := []any{}
			for i := start; i < end; i++ {
				items = append(items, map[string]any{"SK": map[string]any{"S": fmt.Sprintf("TASK#c1#%d", i)}})
			}
			out := map[string]any{"Items": items}
			if end < tasks {
				out["LastEvaluatedKey"] = map[string]any{
					"PK": map[string]any{"S": "b1"},
					"SK": map[string]any{"S": fmt.Sprintf("TASK#c1#%d", end-1)},
				}
			}
			return 200, out
		case "BatchWriteItem":
			reqs := body["RequestItems"].(map[string]any)["Skipli"].([]any)
			if unprocessed && left {
				left = false
				return 200, map[string]any{"UnprocessedItems": map[string]any{"Skipli": reqs[len(reqs)-1:]}}
			}
		}
		return 200, map[string]any{}
	})
}

func deleteCard(t *testing.T, fake *awsfake.Server) error {
	return NewStore(fake.DynamoDB(), "Skipli").Apply(context.Background(),
		Change{Type: "deleteCard", BoardID: "b1", CardID: "c1"})
}

func TestDeleteCardWithManyTasksDeletesThemInBatchesThenTheCard(t *testing.T) {
	fake := deleteFake(t, 130, 50, false)
	if err := deleteCard(t, fake); err != nil {
		t.Fatal(err)
	}

	if calls := fake.Calls("TransactWriteItems"); len(calls) != 0 {
		t.Errorf("got %d transactions, want none", len(calls))
	}
	deleted := map[string]bool{}
	for _, call := range fake.Calls("BatchWriteItem") {
		reqs := call.Body["RequestItems"].(map[string]any)["Skipli"].([]any)
		if len(reqs) > 25 {
			t.Errorf("batch of %d, want at most 25", len(reqs))
		}
		for _, r := range reqs {
			key := r.(map[string]any)["DeleteRequest"].(map[string]any)["Key"].(map[string]any)
			deleted[key["SK"].(map[string]any)["S"].(string)] = true
		}
	}
	if len(deleted) != 130 {
		t.Errorf("deleted %d tasks, want 130", len(deleted))
	}

	calls := fake.Calls()
	last := calls[len(calls)-1]
	if last.Op != "DeleteItem" {
		t.Fatalf("last call is %s, want DeleteItem of the card", last.Op)
	}
	if sk := last.Body["Key"].(map[string]any)["SK"].(map[string]any)["S"]; sk != "CARD#c1" {
		t.Errorf("deleted %v, want CARD#c1", sk)
	}
}

func TestDeleteCardRetriesWhatDynamoDBLeavesUnprocessed(t *testing.T) {
	fake := deleteFake(t, 10, 100, true)
	if err := deleteCard(t, fake); err != nil {
		t.Fatal(err)
	}
	if got := len(fake.Calls("BatchWriteItem")); got != 2 {
		t.Errorf("got %d batch writes, want 2", got)
	}
}

func TestDeleteCardThatIsGoneIsDeleted(t *testing.T) {
	fake := deleteFake(t, 0, 100, false)
	if err := deleteCard(t, fake); err != nil {
		t.Fatal(err)
	}
	if got := len(fake.Calls("BatchWriteItem")); got != 0 {
		t.Errorf("got %d batch writes, want none", got)
	}
}

func TestDeleteCardKeepsTheCardWhenATaskBatchFails(t *testing.T) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		switch op {
		case "Query":
			return 200, map[string]any{"Items": []any{map[string]any{"SK": map[string]any{"S": "TASK#c1#1"}}}}
		case "BatchWriteItem":
			return 500, map[string]any{"__type": "InternalServerError", "message": "boom"}
		}
		return 200, map[string]any{}
	})
	err := deleteCard(t, fake)
	wantStatus(t, err, http.StatusInternalServerError, "")
	if calls := fake.Calls("DeleteItem"); len(calls) != 0 {
		t.Errorf("card deleted although its tasks were not")
	}
}
