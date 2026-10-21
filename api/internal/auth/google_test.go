package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const appURL = "https://app.example.com"

type googleTest struct {
	handler *Handler
	// exchanged is the form of each code exchange the fake Cognito domain got.
	exchanged []url.Values
	status    int
}

func newGoogleTest(t *testing.T) *googleTest {
	gt := &googleTest{status: http.StatusOK}
	domain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		gt.exchanged = append(gt.exchanged, req.PostForm)
		w.WriteHeader(gt.status)
		json.NewEncoder(w).Encode(map[string]string{"id_token": "id1", "refresh_token": "r1"})
	}))
	t.Cleanup(domain.Close)

	gt.handler = NewHandler(nil, "client", nil, nil, nil)
	gt.handler.EnableGoogle(&Google{Domain: domain.URL, AppURL: appURL, Client: domain.Client()})
	return gt
}

// start leaves for Google and returns where it was sent and the flow cookie.
func (gt *googleTest) start(t *testing.T, target string) (*url.URL, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	gt.handler.googleStart(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("start got %d, want 302", rec.Code)
	}
	location, _ := url.Parse(rec.Header().Get("Location"))
	for _, c := range rec.Result().Cookies() {
		if c.Name == googleFlowCookie {
			return location, c
		}
	}
	t.Fatal("start set no flow cookie")
	return nil, nil
}

func (gt *googleTest) callback(query url.Values, flow *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/auth/google/callback?"+query.Encode(), nil)
	if flow != nil {
		req.AddCookie(flow)
	}
	rec := httptest.NewRecorder()
	gt.handler.googleCallback(rec, req)
	return rec
}

func cookies(rec *httptest.ResponseRecorder) map[string]*http.Cookie {
	got := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		got[c.Name] = c
	}
	return got
}

func TestGoogleStartSendsToGoogleWithPKCE(t *testing.T) {
	gt := newGoogleTest(t)

	location, flow := gt.start(t, "/auth/google")

	q := location.Query()
	if location.Path != "/oauth2/authorize" || q.Get("identity_provider") != "Google" || q.Get("client_id") != "client" || q.Get("response_type") != "code" {
		t.Errorf("sent to %s, want Cognito's authorize endpoint for Google", location)
	}
	if q.Get("redirect_uri") != appURL+"/api/auth/google/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if !flow.HttpOnly || !flow.Secure || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/api/auth/google" {
		t.Errorf("flow cookie %+v, want HttpOnly, Secure, Lax on /api/auth/google", flow)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(flow)
	saved, _ := readGoogleFlow(req)
	challenge := sha256.Sum256([]byte(saved.Verifier))
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Errorf("code challenge doesn't match the saved verifier")
	}
	if len(saved.Verifier) < 43 || q.Get("state") != saved.State {
		t.Errorf("flow %+v, want a 43+ character verifier and the sent state", saved)
	}
}

func TestGoogleCallbackExchangesTheCodeAndSetsSessionCookies(t *testing.T) {
	gt := newGoogleTest(t)
	location, flow := gt.start(t, "/auth/google")

	rec := gt.callback(url.Values{"code": {"c1"}, "state": {location.Query().Get("state")}}, flow)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != appURL+"/signed-in" {
		t.Fatalf("got %d to %q, want 302 to /signed-in", rec.Code, rec.Header().Get("Location"))
	}
	if len(gt.exchanged) != 1 {
		t.Fatalf("got %d exchanges, want 1", len(gt.exchanged))
	}
	form := gt.exchanged[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(flow)
	saved, _ := readGoogleFlow(req)
	if form.Get("code") != "c1" || form.Get("code_verifier") != saved.Verifier || form.Get("grant_type") != "authorization_code" || form.Get("redirect_uri") != appURL+"/api/auth/google/callback" {
		t.Errorf("exchanged %v", form)
	}

	got := cookies(rec)
	if c := got["id_token"]; c == nil || c.Value != "id1" || c.Path != "/api" || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("id_token cookie %+v", c)
	}
	if c := got["refresh_token"]; c == nil || c.Value != "r1" || c.Path != "/api/auth" || !c.HttpOnly {
		t.Errorf("refresh_token cookie %+v", c)
	}
	if c := got[googleFlowCookie]; c == nil || c.MaxAge >= 0 {
		t.Errorf("flow cookie not cleared: %+v", c)
	}
}

func TestGoogleCallbackWithAnotherBrowsersStateIsRefused(t *testing.T) {
	gt := newGoogleTest(t)
	_, flow := gt.start(t, "/auth/google")

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"wrong state": gt.callback(url.Values{"code": {"c1"}, "state": {"forged"}}, flow),
		"no cookie":   gt.callback(url.Values{"code": {"c1"}, "state": {"forged"}}, nil),
	} {
		if rec.Header().Get("Location") != appURL+"/login?error=google" || len(gt.exchanged) != 0 || cookies(rec)["id_token"] != nil {
			t.Errorf("%s: got %q after %d exchanges, want back to login without a session", name, rec.Header().Get("Location"), len(gt.exchanged))
		}
	}
}

func TestGoogleCallbackRetriesOnceAfterLinking(t *testing.T) {
	gt := newGoogleTest(t)
	linkError := func(location *url.URL) url.Values {
		return url.Values{
			"error":             {"invalid_request"},
			"error_description": {"PreSignUp failed with error Already found an entry for username google_1234. "},
			"state":             {location.Query().Get("state")},
		}
	}

	location, flow := gt.start(t, "/auth/google")
	rec := gt.callback(linkError(location), flow)
	if rec.Header().Get("Location") != appURL+"/api/auth/google?retry" {
		t.Fatalf("got %q, want sent to sign in again", rec.Header().Get("Location"))
	}

	location, flow = gt.start(t, "/auth/google?retry")
	rec = gt.callback(linkError(location), flow)
	if !strings.HasPrefix(rec.Header().Get("Location"), appURL+"/login?error=") {
		t.Errorf("second failure got %q, want back to login", rec.Header().Get("Location"))
	}
}

func TestGoogleCallbackWhenCancelledOrExchangeFails(t *testing.T) {
	gt := newGoogleTest(t)

	location, flow := gt.start(t, "/auth/google")
	rec := gt.callback(url.Values{"error": {"access_denied"}, "state": {location.Query().Get("state")}}, flow)
	if rec.Header().Get("Location") != appURL+"/login?error=google_cancelled" {
		t.Errorf("cancelled got %q", rec.Header().Get("Location"))
	}

	gt.status = http.StatusBadRequest
	location, flow = gt.start(t, "/auth/google")
	rec = gt.callback(url.Values{"code": {"c1"}, "state": {location.Query().Get("state")}}, flow)
	if rec.Header().Get("Location") != appURL+"/login?error=google" || cookies(rec)["id_token"] != nil {
		t.Errorf("failed exchange got %q with cookies %v", rec.Header().Get("Location"), cookies(rec))
	}
}

func TestGoogleIsOffUnlessEnabled(t *testing.T) {
	h := NewHandler(nil, "client", nil, nil, nil)
	rec := httptest.NewRecorder()
	h.googleStart(rec, httptest.NewRequest("GET", "/auth/google", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("start got %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.providers(rec, httptest.NewRequest("GET", "/auth/providers", nil))
	if strings.TrimSpace(rec.Body.String()) != `{"google":false}` {
		t.Errorf("providers = %s", rec.Body.String())
	}
}
