package board

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

func newInviteTest(t *testing.T) (*awsfake.Server, *Handler) {
	fake := awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "GetItem":
			return 200, map[string]any{"Item": map[string]any{"PK": map[string]any{"S": "b1"}}}
		case "AdminCreateUser":
			return 200, map[string]any{"User": map[string]any{"Attributes": []any{
				map[string]any{"Name": "sub", "Value": "u2"},
				map[string]any{"Name": "email", "Value": body["Username"]},
			}}}
		}
		return 200, map[string]any{}
	})
	idp := fake.Cognito()
	return fake, NewHandler(fake.DynamoDB(), "Skipli", nil, auth.NewDirectory(idp, "pool"), nil, auth.CanReceiveMail, broadcast.Discard{})
}

func invite(h *Handler, email string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/boards/b1/invite", strings.NewReader(`{"email":"`+email+`"}`))
	req.SetPathValue("id", "b1")
	req = req.WithContext(auth.ContextWithUser(req.Context(), auth.User{ID: "u1", Email: "owner@example.com"}))
	rec := httptest.NewRecorder()
	h.invite(rec, req)
	return rec
}

func TestInviteRejectsMalformedEmail(t *testing.T) {
	fake, h := newInviteTest(t)

	for _, email := range []string{"not-an-email", "Eve <eve@example.com>"} {
		if rec := invite(h, email); rec.Code != http.StatusBadRequest {
			t.Errorf("invite %q: got %d, want 400", email, rec.Code)
		}
	}
	if calls := fake.Calls("AdminCreateUser"); len(calls) != 0 {
		t.Errorf("malformed invite created Cognito users: %v", calls)
	}
}

func TestInviteRejectsUndeliverableEmail(t *testing.T) {
	fake, h := newInviteTest(t)
	h.canReceiveMail = func(context.Context, string) bool { return false }

	if rec := invite(h, "nobody@nowhere.invalid"); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if calls := fake.Calls("AdminCreateUser"); len(calls) != 0 {
		t.Errorf("undeliverable invite created Cognito users: %v", calls)
	}
}

func TestInviteNormalizesEmail(t *testing.T) {
	fake, h := newInviteTest(t)
	var checked string
	h.canReceiveMail = func(_ context.Context, email string) bool {
		checked = email
		return true
	}

	if rec := invite(h, "  Friend@Example.COM "); rec.Code != http.StatusNoContent {
		t.Fatalf("got %d %q, want 204", rec.Code, rec.Body.String())
	}
	if checked != "friend@example.com" {
		t.Errorf("checked %q, want friend@example.com", checked)
	}
	created := fake.Calls("AdminCreateUser")
	if len(created) != 1 || created[0].Body["Username"] != "friend@example.com" {
		t.Errorf("AdminCreateUser calls = %v, want one for friend@example.com", created)
	}
}
