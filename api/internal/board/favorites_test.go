package board

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

func putAsU1(t *testing.T, path, body string, respond awsfake.Respond, serve func(*Handler) http.HandlerFunc) (*awsfake.Server, *httptest.ResponseRecorder) {
	fake := awsfake.New(t, respond)
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, broadcast.Discard{})
	req := httptest.NewRequest("PUT", path, strings.NewReader(body))
	req.SetPathValue("id", "b1")
	req = req.WithContext(auth.ContextWithUser(req.Context(), auth.User{ID: "u1", Email: "me@example.com"}))
	rec := httptest.NewRecorder()
	serve(h)(rec, req)
	return fake, rec
}

func TestOrderFavoritesWritesFavoriteOrderFromOne(t *testing.T) {
	fake, rec := putAsU1(t, "/boards/favorites/order", `{"boardIds":["b2","b1"]}`, ok,
		func(h *Handler) http.HandlerFunc { return h.orderFavorites })
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d %q, want 204", rec.Code, rec.Body.String())
	}
	calls := fake.Calls("TransactWriteItems")
	if len(calls) != 1 {
		t.Fatalf("got %d transactions, want 1", len(calls))
	}
	items := calls[0].Body["TransactItems"].([]any)
	for i, want := range []struct{ pk, order string }{{"b2", "1"}, {"b1", "2"}} {
		u := items[i].(map[string]any)["Update"].(map[string]any)
		key := u["Key"].(map[string]any)
		if key["PK"].(map[string]any)["S"] != want.pk || key["SK"].(map[string]any)["S"] != "MEMBER#u1" {
			t.Errorf("item %d key = %v, want %s / MEMBER#u1", i, key, want.pk)
		}
		if name := u["ExpressionAttributeNames"].(map[string]any)["#order"]; name != "FavoriteOrder" {
			t.Errorf("item %d writes %v, want FavoriteOrder", i, name)
		}
		if got := u["ExpressionAttributeValues"].(map[string]any)[":order"].(map[string]any)["N"]; got != want.order {
			t.Errorf("item %d order = %v, want %s", i, got, want.order)
		}
	}
}

func TestOrderFavoritesRejectsDuplicates(t *testing.T) {
	_, rec := putAsU1(t, "/boards/favorites/order", `{"boardIds":["b1","b1"]}`, ok,
		func(h *Handler) http.HandlerFunc { return h.orderFavorites })
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestOrderFavoritesRefusesABoardTheCallerIsNotIn(t *testing.T) {
	_, rec := putAsU1(t, "/boards/favorites/order", `{"boardIds":["b1"]}`,
		func(op string, _ map[string]any) (int, any) {
			return 400, map[string]any{
				"__type":              "TransactionCanceledException",
				"CancellationReasons": []any{map[string]any{"Code": "ConditionalCheckFailed"}},
			}
		}, func(h *Handler) http.HandlerFunc { return h.orderFavorites })
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestOrderBoardsStillWritesOrderFromZero(t *testing.T) {
	fake, _ := putAsU1(t, "/boards/order", `{"boardIds":["b1"]}`, ok,
		func(h *Handler) http.HandlerFunc { return h.order })
	u := fake.Calls("TransactWriteItems")[0].Body["TransactItems"].([]any)[0].(map[string]any)["Update"].(map[string]any)
	if u["ExpressionAttributeNames"].(map[string]any)["#order"] != "Order" ||
		u["ExpressionAttributeValues"].(map[string]any)[":order"].(map[string]any)["N"] != "0" {
		t.Errorf("got %v", u)
	}
}

func TestFavoritingGoesLastAndUnfavoritingClearsTheOrder(t *testing.T) {
	fake, rec := putAsU1(t, "/boards/b1/favorite", `{"favorite":true}`, ok,
		func(h *Handler) http.HandlerFunc { return h.favorite })
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", rec.Code)
	}
	body := fake.Calls("UpdateItem")[0].Body
	if !strings.Contains(body["UpdateExpression"].(string), "#order = :order") {
		t.Errorf("favoriting set no order: %v", body["UpdateExpression"])
	}
	if n := body["ExpressionAttributeValues"].(map[string]any)[":order"].(map[string]any)["N"].(string); len(n) < 12 {
		t.Errorf("order %q is not a millisecond timestamp", n)
	}

	fake, _ = putAsU1(t, "/boards/b1/favorite", `{"favorite":false}`, ok,
		func(h *Handler) http.HandlerFunc { return h.favorite })
	if got := fake.Calls("UpdateItem")[0].Body["UpdateExpression"]; !strings.Contains(got.(string), "REMOVE #order") {
		t.Errorf("unfavoriting kept the order: %v", got)
	}
}
