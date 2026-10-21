package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleFlowCookie = "google_flow"
	googleStartPath  = "/api/auth/google"
	googleCallback   = "/api/auth/google/callback"
	// Cognito's first sign-in after PreSignUp links an account fails with
	// this, though the link holds; signing in again goes through.
	linkedOnSignUp = "Already found an entry for username"
)

// Google signs people in with Google through the user pool's hosted OAuth
// endpoints, using the authorization code flow with PKCE. The API exchanges
// the code itself, so the tokens only ever reach the browser as the same
// HttpOnly cookies an email code sets.
type Google struct {
	// Domain is the user pool's Cognito domain, such as
	// https://skipli-dev.auth.us-east-1.amazoncognito.com.
	Domain string
	// AppURL is the UI's origin, which serves the API under /api.
	AppURL string
	Client *http.Client
}

func (g *Google) redirectURI() string {
	return g.AppURL + googleCallback
}

// googleFlow is what the browser keeps between leaving for Google and coming
// back: the state that ties the callback to this browser, the PKCE verifier,
// and whether this is already the retry after linking.
type googleFlow struct {
	State    string `json:"state"`
	Verifier string `json:"verifier"`
	Retry    bool   `json:"retry,omitempty"`
}

func (h *Handler) providers(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"google": h.google != nil})
}

func (h *Handler) googleStart(w http.ResponseWriter, req *http.Request) {
	if h.google == nil {
		http.NotFound(w, req)
		return
	}

	flow := googleFlow{
		State:    rand.Text(),
		Verifier: rand.Text() + rand.Text(),
		Retry:    req.URL.Query().Has("retry"),
	}
	raw, _ := json.Marshal(flow)
	// Lax, not Strict: the browser has to send it on the redirect back from
	// Google, a navigation another site started.
	http.SetCookie(w, &http.Cookie{
		Name:     googleFlowCookie,
		Value:    base64.RawURLEncoding.EncodeToString(raw),
		Path:     googleStartPath,
		MaxAge:   10 * 60,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	challenge := sha256.Sum256([]byte(flow.Verifier))
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.clientID},
		"redirect_uri":          {h.google.redirectURI()},
		"identity_provider":     {googleProvider},
		"scope":                 {"openid email profile"},
		"state":                 {flow.State},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, req, h.google.Domain+"/oauth2/authorize?"+query.Encode(), http.StatusFound)
}

func (h *Handler) googleCallback(w http.ResponseWriter, req *http.Request) {
	if h.google == nil {
		http.NotFound(w, req)
		return
	}
	fail := func(reason string) {
		http.Redirect(w, req, h.google.AppURL+"/login?error="+url.QueryEscape(reason), http.StatusFound)
	}

	flow, ok := readGoogleFlow(req)
	http.SetCookie(w, &http.Cookie{Name: googleFlowCookie, Path: googleStartPath, MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	query := req.URL.Query()
	if !ok || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(flow.State)) != 1 {
		fail("google")
		return
	}

	if errCode := query.Get("error"); errCode != "" {
		description := query.Get("error_description")
		if strings.Contains(description, linkedOnSignUp) && !flow.Retry {
			http.Redirect(w, req, h.google.AppURL+googleStartPath+"?retry", http.StatusFound)
			return
		}
		if errCode == "access_denied" && description == "" {
			fail("google_cancelled")
			return
		}
		log.Printf("google sign-in failed: %s: %s", errCode, description)
		fail("google")
		return
	}

	tokens, err := h.google.exchange(req, h.clientID, query.Get("code"), flow.Verifier)
	if err != nil {
		log.Printf("google sign-in: %v", err)
		fail("google")
		return
	}

	setCookie(w, idTokenCookie, tokens.IDToken, idTokenPath, 60*60)
	setCookie(w, refreshTokenCookie, tokens.RefreshToken, refreshPath, 30*24*60*60)
	http.Redirect(w, req, h.google.AppURL+"/signed-in", http.StatusFound)
}

func readGoogleFlow(req *http.Request) (googleFlow, bool) {
	value, ok := cookieValue(req, googleFlowCookie)
	if !ok {
		return googleFlow{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return googleFlow{}, false
	}
	var flow googleFlow
	if json.Unmarshal(raw, &flow) != nil || flow.State == "" || flow.Verifier == "" {
		return googleFlow{}, false
	}
	return flow, true
}

type oauthTokens struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
}

func (g *Google) exchange(req *http.Request, clientID, code, verifier string) (oauthTokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {g.redirectURI()},
		"code_verifier": {verifier},
	}
	tokenReq, err := http.NewRequestWithContext(req.Context(), "POST", g.Domain+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokens{}, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(tokenReq)
	if err != nil {
		return oauthTokens{}, fmt.Errorf("exchange code: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var body struct {
			Error string `json:"error"`
		}
		json.NewDecoder(res.Body).Decode(&body)
		return oauthTokens{}, fmt.Errorf("exchange code: %s %s", res.Status, body.Error)
	}

	var tokens oauthTokens
	if err := json.NewDecoder(res.Body).Decode(&tokens); err != nil {
		return oauthTokens{}, fmt.Errorf("exchange code: %w", err)
	}
	if tokens.IDToken == "" || tokens.RefreshToken == "" {
		return oauthTokens{}, fmt.Errorf("exchange code: tokens missing")
	}
	return tokens, nil
}
