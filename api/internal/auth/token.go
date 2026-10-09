package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	idTokenCookie      = "id_token"
	refreshTokenCookie = "refresh_token"
	refreshPath        = "/api/auth/refresh"
)

func setCookie(w http.ResponseWriter, name, value, path string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func cookieValue(req *http.Request, name string) (string, bool) {
	c, err := req.Cookie(name)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

type TokenVerifier struct {
	keys     keyfunc.Keyfunc
	issuer   string
	clientID string
}

func NewTokenVerifier(ctx context.Context, region, poolID, clientID string) (*TokenVerifier, error) {
	issuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", region, poolID)
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{issuer + "/.well-known/jwks.json"})
	if err != nil {
		return nil, err
	}
	return &TokenVerifier{keys: keys, issuer: issuer, clientID: clientID}, nil
}

func (t *TokenVerifier) Verify(tokenString string) (User, error) {
	token, err := jwt.Parse(tokenString, t.keys.Keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(t.issuer),
		jwt.WithAudience(t.clientID),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return User{}, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["token_use"] != "id" {
		return User{}, fmt.Errorf("invalid token claims")
	}

	id, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	if id == "" || email == "" {
		return User{}, fmt.Errorf("invalid token claims")
	}

	return User{ID: id, Email: email}, nil
}

func (t *TokenVerifier) VerifyCookies(cookieHeader string) (User, error) {
	req := &http.Request{Header: http.Header{"Cookie": {cookieHeader}}}
	tokenString, ok := cookieValue(req, idTokenCookie)
	if !ok {
		return User{}, fmt.Errorf("missing token")
	}
	return t.Verify(tokenString)
}

type contextKey int

const userContextKey contextKey = iota

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey).(User)
	return user, ok
}

func (t *TokenVerifier) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		tokenString, ok := cookieValue(req, idTokenCookie)
		if !ok {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		user, err := t.Verify(tokenString)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(req.Context(), userContextKey, user)
		next(w, req.WithContext(ctx))
	}
}
