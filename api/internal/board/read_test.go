package board

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/profile"
)

func s(v string) map[string]any { return map[string]any{"S": v} }
func n(v string) map[string]any { return map[string]any{"N": v} }

func item(sk string, attrs map[string]any) map[string]any {
	attrs["PK"] = s("b1")
	attrs["SK"] = s(sk)
	return attrs
}

// newReadTest serves the board's partition as two Query pages, in key
// order as DynamoDB would, with the cards and tasks out of their Order.
func newReadTest(t *testing.T, pages ...[]any) (*awsfake.Server, *Handler) {
	fake := awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "Query":
			page := 0
			if body["ExclusiveStartKey"] != nil {
				page = 1
			}
			out := map[string]any{"Items": pages[page]}
			if page+1 < len(pages) {
				out["LastEvaluatedKey"] = map[string]any{"PK": s("b1"), "SK": s("page")}
			}
			return 200, out
		case "BatchGetItem":
			return 200, map[string]any{"Responses": map[string]any{"Skipli": []any{
				map[string]any{"PK": s("USER#u2"), "SK": s("PROFILE"), "Name": s("Bea")},
			}}}
		}
		return 200, map[string]any{}
	})
	db := fake.DynamoDB()
	return fake, NewHandler(db, "Skipli", nil, nil, profile.NewStore(db, "Skipli"), nil, nil)
}

func task(card, id, order, status string) map[string]any {
	return item("TASK#"+card+"#"+id, map[string]any{
		"ID": s(id), "CardID": s(card), "Name": s(id), "Description": s(""), "Status": s(status), "Order": n(order),
	})
}

func TestReadGivesTheWholeBoardInOrder(t *testing.T) {
	fake, h := newReadTest(t,
		[]any{
			item("CARD#a", map[string]any{"ID": s("a"), "Name": s("Second"), "Description": s(""), "Order": n("2000")}),
			item("CARD#b", map[string]any{"ID": s("b"), "Name": s("First"), "Description": s("d"), "Order": n("1000")}),
			item("CARD#c", map[string]any{"ID": s("c"), "Name": s("Empty"), "Description": s(""), "Order": n("3000")}),
			item("MEMBER#u1", map[string]any{"Member": s("u1"), "Email": s("ann@example.com")}),
		},
		[]any{
			item("MEMBER#u2", map[string]any{"Member": s("u2"), "Email": s("bea@example.com")}),
			item("META", map[string]any{"ID": s("b1"), "Owner": s("u1"), "Name": s("Board"), "Description": s("About")}),
			task("a", "t1", "2000", "done"),
			task("a", "t2", "1000", "pending"),
			task("b", "t3", "1000", "pending"),
			// A task whose card is gone isn't shown.
			task("gone", "t4", "1000", "pending"),
		},
	)

	whole, ok, err := h.read(context.Background(), "b1")
	if err != nil || !ok {
		t.Fatalf("read = %v, %v", ok, err)
	}
	got, _ := json.Marshal(whole)
	want := `{"id":"b1","owner":"u1","name":"Board","description":"About",` +
		`"members":[{"id":"u1","email":"ann@example.com","name":"ann"},{"id":"u2","email":"bea@example.com","name":"Bea"}],` +
		`"cards":[` +
		`{"id":"b","name":"First","description":"d","tasks":[{"id":"t3","cardId":"b","name":"t3","description":"","status":"pending"}]},` +
		`{"id":"a","name":"Second","description":"","tasks":[` +
		`{"id":"t2","cardId":"a","name":"t2","description":"","status":"pending"},` +
		`{"id":"t1","cardId":"a","name":"t1","description":"","status":"done"}]},` +
		`{"id":"c","name":"Empty","description":"","tasks":[]}]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if queries := fake.Calls("Query"); len(queries) != 2 {
		t.Errorf("made %d Query calls, want 2 (one per page)", len(queries))
	}
}

func TestReadMissingBoard(t *testing.T) {
	_, h := newReadTest(t, []any{})

	req := httptest.NewRequest("GET", "/boards/b1", nil)
	req.SetPathValue("id", "b1")
	rec := httptest.NewRecorder()
	h.get(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}
