package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

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

func (t *TokenVerifier) Verify(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, t.keys.Keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(t.issuer),
		jwt.WithAudience(t.clientID),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["token_use"] != "id" {
		return "", fmt.Errorf("invalid token claims")
	}

	email, ok := claims["email"].(string)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	return email, nil
}

type contextKey int

const emailContextKey contextKey = iota

func EmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(emailContextKey).(string)
	return email, ok
}

func (t *TokenVerifier) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		tokenString, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		email, err := t.Verify(tokenString)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(req.Context(), emailContextKey, email)
		next(w, req.WithContext(ctx))
	}
}
