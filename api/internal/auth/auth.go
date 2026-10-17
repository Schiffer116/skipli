package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

type Handler struct {
	idp            *cip.Client
	clientID       string
	users          *Directory
	tokens         *TokenVerifier
	cooldown       *Cooldown
	canReceiveMail func(context.Context, string) bool
}

func NewHandler(idp *cip.Client, clientID string, users *Directory, tokens *TokenVerifier, cooldown *Cooldown) *Handler {
	return &Handler{
		idp:            idp,
		clientID:       clientID,
		users:          users,
		tokens:         tokens,
		cooldown:       cooldown,
		canReceiveMail: CanReceiveMail,
	}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /auth/send", h.sendVerificationEmail)
	router.HandleFunc("POST /auth/verify", h.verify)
	router.HandleFunc("POST /auth/refresh", h.refresh)
	router.HandleFunc("POST /auth/signout", h.signOut)
}

type SendVerificationEmailRequest struct {
	Email string `json:"email"`
}

type SendVerificationEmailResponse struct {
	Session string `json:"session"`
}

func (h *Handler) sendVerificationEmail(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var r SendVerificationEmailRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	email := NormalizeEmail(r.Email)
	if !h.canReceiveMail(ctx, email) {
		http.Error(w, "email address can't receive mail", http.StatusBadRequest)
		return
	}

	started, remaining, err := h.cooldown.Start(ctx, email)
	if err != nil {
		log.Printf("failed to start send cooldown: %v", err)
		http.Error(w, "failed to send verification email", http.StatusInternalServerError)
		return
	}
	if !started {
		w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())))
		http.Error(w, "a verification email was just sent; wait before requesting another", http.StatusTooManyRequests)
		return
	}

	session, err := h.startSignIn(ctx, email)
	if err != nil {
		log.Printf("failed to send verification email: %v", err)
		if err := h.cooldown.Cancel(ctx, email); err != nil {
			log.Printf("failed to cancel send cooldown: %v", err)
		}
		http.Error(w, "failed to send verification email", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SendVerificationEmailResponse{Session: session})
}

func (h *Handler) startSignIn(ctx context.Context, email string) (string, error) {
	if _, err := h.users.Ensure(ctx, email); err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	out, err := h.idp.InitiateAuth(ctx, &cip.InitiateAuthInput{
		ClientId: aws.String(h.clientID),
		AuthFlow: types.AuthFlowTypeUserAuth,
		AuthParameters: map[string]string{
			"USERNAME":            email,
			"PREFERRED_CHALLENGE": string(types.ChallengeNameTypeEmailOtp),
		},
	})
	if err != nil {
		return "", fmt.Errorf("start sign-in: %w", err)
	}
	if out.Session == nil {
		return "", errors.New("start sign-in: no session returned")
	}
	return *out.Session, nil
}

// NormalizeEmail trims and lowercases an address. User pool usernames are
// case-insensitive, so this keeps per-address state such as the send cooldown
// from being sidestepped by changing case.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CanReceiveMail reports whether email is a bare, well-formed address whose
// domain accepts mail. DNS failures other than "not found" count as
// deliverable so an outage doesn't block sign-in.
func CanReceiveMail(ctx context.Context, email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	_, domain, _ := strings.Cut(addr.Address, "@")

	mx, err := net.DefaultResolver.LookupMX(ctx, domain)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && !dnsErr.IsNotFound {
		return true
	}
	return err == nil && len(mx) > 0 && mx[0].Host != "."
}

type VerifyRequest struct {
	Email   string `json:"email"`
	Code    string `json:"code"`
	Session string `json:"session"`
}

func (h *Handler) verify(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var r VerifyRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	out, err := h.idp.RespondToAuthChallenge(ctx, &cip.RespondToAuthChallengeInput{
		ClientId:      aws.String(h.clientID),
		ChallengeName: types.ChallengeNameTypeEmailOtp,
		Session:       aws.String(r.Session),
		ChallengeResponses: map[string]string{
			"USERNAME":       r.Email,
			"EMAIL_OTP_CODE": r.Code,
		},
	})
	var mismatch *types.CodeMismatchException
	var expired *types.ExpiredCodeException
	var notAuthorized *types.NotAuthorizedException
	if errors.As(err, &mismatch) || errors.As(err, &expired) || errors.As(err, &notAuthorized) {
		http.Error(w, "invalid email or verification code", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("failed to verify code: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out.AuthenticationResult == nil {
		http.Error(w, "invalid email or verification code", http.StatusUnauthorized)
		return
	}

	setCookie(w, idTokenCookie, *out.AuthenticationResult.IdToken, idTokenPath, 60*60)
	setCookie(w, refreshTokenCookie, *out.AuthenticationResult.RefreshToken, refreshPath, 30*24*60*60)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) refresh(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	refreshToken, ok := cookieValue(req, refreshTokenCookie)
	if !ok {
		http.Error(w, "missing refresh token", http.StatusUnauthorized)
		return
	}

	out, err := h.idp.InitiateAuth(ctx, &cip.InitiateAuthInput{
		ClientId:       aws.String(h.clientID),
		AuthFlow:       types.AuthFlowTypeRefreshTokenAuth,
		AuthParameters: map[string]string{"REFRESH_TOKEN": refreshToken},
	})
	var notAuthorized *types.NotAuthorizedException
	if errors.As(err, &notAuthorized) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("failed to refresh token: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out.AuthenticationResult == nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	setCookie(w, idTokenCookie, *out.AuthenticationResult.IdToken, idTokenPath, 60*60)
	w.WriteHeader(http.StatusNoContent)
}

// signOut ends the session on this device: it revokes the refresh token
// with Cognito, so it can't mint new ID tokens anywhere, and clears both
// cookies. It succeeds even without a session, or if revoking fails, so a
// device can always get back to a signed-out state.
func (h *Handler) signOut(w http.ResponseWriter, req *http.Request) {
	if refreshToken, ok := cookieValue(req, refreshTokenCookie); ok {
		_, err := h.idp.RevokeToken(req.Context(), &cip.RevokeTokenInput{
			ClientId: aws.String(h.clientID),
			Token:    aws.String(refreshToken),
		})
		if err != nil {
			log.Printf("failed to revoke refresh token: %v", err)
		}
	}

	setCookie(w, idTokenCookie, "", idTokenPath, -1)
	setCookie(w, refreshTokenCookie, "", refreshPath, -1)
	setCookie(w, refreshTokenCookie, "", legacyRefreshPath, -1)
	w.WriteHeader(http.StatusNoContent)
}
