package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

func signOut(t *testing.T, cookies []*http.Cookie, revoke func() (int, any)) (*awsfake.Server, *httptest.ResponseRecorder) {
	fake := awsfake.New(t, func(op string, _ map[string]any) (int, any) { return revoke() })
	h := NewHandler(fake.Cognito(), "client", nil, nil, nil, nil)
	req := httptest.NewRequest("POST", "/auth/signout", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.signOut(rec, req)
	return fake, rec
}

func cleared(rec *httptest.ResponseRecorder) map[string]bool {
	paths := map[string]bool{}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 && c.Value == "" {
			paths[c.Name+" "+c.Path] = true
		}
	}
	return paths
}

func TestSignOutRevokesTheRefreshTokenAndClearsCookies(t *testing.T) {
	fake, rec := signOut(t, []*http.Cookie{{Name: "refresh_token", Value: "r1"}}, func() (int, any) { return 200, map[string]any{} })
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", rec.Code)
	}

	revokes := fake.Calls("RevokeToken")
	if len(revokes) != 1 || revokes[0].Body["Token"] != "r1" || revokes[0].Body["ClientId"] != "client" {
		t.Errorf("revoked %v, want r1 for client", revokes)
	}
	for _, want := range []string{"id_token /api", "refresh_token /api/auth", "refresh_token /api/auth/refresh"} {
		if !cleared(rec)[want] {
			t.Errorf("did not clear %s; cleared %v", want, cleared(rec))
		}
	}
}

func TestSignOutSucceedsWhenRevokingFails(t *testing.T) {
	_, rec := signOut(t, []*http.Cookie{{Name: "refresh_token", Value: "r1"}}, func() (int, any) {
		return 500, map[string]any{"__type": "InternalErrorException", "message": "down"}
	})
	if rec.Code != http.StatusNoContent || !cleared(rec)["id_token /api"] {
		t.Fatalf("got %d with cleared %v, want 204 and cookies cleared", rec.Code, cleared(rec))
	}
}

func TestSignOutWithoutASessionStillClearsCookies(t *testing.T) {
	fake, rec := signOut(t, nil, func() (int, any) { return 200, map[string]any{} })
	if rec.Code != http.StatusNoContent || !cleared(rec)["id_token /api"] {
		t.Fatalf("got %d with cleared %v", rec.Code, cleared(rec))
	}
	if calls := fake.Calls("RevokeToken"); len(calls) != 0 {
		t.Errorf("revoked without a token: %v", calls)
	}
}

func TestRefreshDuringACognitoOutageIsAnErrorNotAPanic(t *testing.T) {
	fake := awsfake.New(t, func(string, map[string]any) (int, any) {
		return 500, map[string]any{"__type": "InternalErrorException", "message": "down"}
	})
	h := NewHandler(fake.Cognito(), "client", nil, nil, nil, nil)
	req := httptest.NewRequest("POST", "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "r1"})
	rec := httptest.NewRecorder()
	h.refresh(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
}
