package board

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

func TestRenameOfADeletedBoardIsNotFound(t *testing.T) {
	fake := awsfake.New(t, func(string, map[string]any) (int, any) {
		return 400, map[string]any{"__type": "ConditionalCheckFailedException", "message": "missing"}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, broadcast.Discard{})
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

type sentMessages []broadcast.Message

func (s *sentMessages) Broadcast(_ context.Context, boardID string, m broadcast.Message) {
	*s = append(*s, m)
}

func TestRenameIsBroadcastUnderTheSendersChangeID(t *testing.T) {
	fake := awsfake.New(t, func(string, map[string]any) (int, any) { return 200, map[string]any{} })
	var sent sentMessages
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, &sent)
	req := httptest.NewRequest("PUT", "/boards/b1", strings.NewReader(`{"name":"Plans","description":"Q4"}`))
	req.SetPathValue("id", "b1")
	req.Header.Set(broadcast.ChangeIDHeader, "x1")
	rec := httptest.NewRecorder()
	h.put(rec, req)

	if rec.Code != http.StatusNoContent || len(sent) != 1 || sent[0].Type != broadcast.BoardUpdated || sent[0].ID != "x1" ||
		*sent[0].Board.Name != "Plans" || *sent[0].Board.Description != "Q4" {
		t.Fatalf("got %d, sent %+v", rec.Code, sent)
	}
}

func TestRenameLeavesTheDescriptionAlone(t *testing.T) {
	fake := awsfake.New(t, func(string, map[string]any) (int, any) { return 200, map[string]any{} })
	var sent sentMessages
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, &sent)
	req := httptest.NewRequest("PUT", "/boards/b1", strings.NewReader(`{"name":"Plans"}`))
	req.SetPathValue("id", "b1")
	rec := httptest.NewRecorder()
	h.put(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	update := fake.Calls("UpdateItem")[0].Body
	if names := update["ExpressionAttributeNames"].(map[string]any); len(names) != 1 || names["#0"] != "Name" {
		t.Errorf("update sets %v, want only Name", names)
	}
	if body, _ := json.Marshal(sent[0].Board); string(body) != `{"name":"Plans"}` {
		t.Errorf("broadcast %s, want only the name", body)
	}
}

func TestUpdateWithNothingToChangeIsRefused(t *testing.T) {
	h := NewHandler(nil, "Skipli", nil, nil, nil, nil, broadcast.Discard{})
	req := httptest.NewRequest("PUT", "/boards/b1", strings.NewReader(`{}`))
	req.SetPathValue("id", "b1")
	rec := httptest.NewRecorder()
	h.put(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestDeletingABoardTellsItsConnectionsAfterItIsGone(t *testing.T) {
	var order []string
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) {
		order = append(order, op)
		if op == "GetItem" {
			return 200, map[string]any{"Item": map[string]any{"Owner": map[string]any{"S": "u1"}}}
		}
		if op == "Query" {
			return 200, map[string]any{"Items": []any{map[string]any{"PK": map[string]any{"S": "b1"}, "SK": map[string]any{"S": "META"}}}}
		}
		return 200, map[string]any{}
	})
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, nil, nil, nil, broadcasterFunc(func() { order = append(order, "Broadcast") }))
	req := httptest.NewRequest("DELETE", "/boards/b1", nil)
	req.SetPathValue("id", "b1")
	req = req.WithContext(auth.ContextWithUser(req.Context(), auth.User{ID: "u1"}))
	rec := httptest.NewRecorder()
	h.delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if last := order[len(order)-1]; last != "Broadcast" || order[len(order)-2] != "BatchWriteItem" {
		t.Errorf("calls %v, want the broadcast after the board was deleted", order)
	}
}

type broadcasterFunc func()

func (f broadcasterFunc) Broadcast(context.Context, string, broadcast.Message) { f() }
