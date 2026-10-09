package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/mail"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

type Handler struct {
	idp      *cip.Client
	clientID string
	users    *Directory
	tokens   *TokenVerifier
}

func NewHandler(idp *cip.Client, clientID string, users *Directory, tokens *TokenVerifier) *Handler {
	return &Handler{idp: idp, clientID: clientID, users: users, tokens: tokens}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /auth/send", h.sendVerificationEmail)
	router.HandleFunc("POST /auth/verify", h.verify)
	router.HandleFunc("POST /auth/refresh", h.refresh)
	router.HandleFunc("GET /auth/email", h.getEmailFromJwt)
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

	if !canReceiveMail(ctx, r.Email) {
		http.Error(w, "email address can't receive mail", http.StatusBadRequest)
		return
	}

	if _, err := h.users.Ensure(ctx, r.Email); err != nil {
		log.Printf("failed to create user: %v", err)
		http.Error(w, "failed to send verification email", http.StatusInternalServerError)
		return
	}

	out, err := h.idp.InitiateAuth(ctx, &cip.InitiateAuthInput{
		ClientId: aws.String(h.clientID),
		AuthFlow: types.AuthFlowTypeUserAuth,
		AuthParameters: map[string]string{
			"USERNAME":            r.Email,
			"PREFERRED_CHALLENGE": string(types.ChallengeNameTypeEmailOtp),
		},
	})
	if err != nil || out.Session == nil {
		log.Printf("failed to start sign-in: %v", err)
		http.Error(w, "failed to send verification email", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SendVerificationEmailResponse{Session: *out.Session})
}

func canReceiveMail(ctx context.Context, email string) bool {
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

	setCookie(w, idTokenCookie, *out.AuthenticationResult.IdToken, "/api", 60*60)
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
	if errors.As(err, &notAuthorized) || out.AuthenticationResult == nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("failed to refresh token: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	setCookie(w, idTokenCookie, *out.AuthenticationResult.IdToken, "/api", 60*60)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getEmailFromJwt(w http.ResponseWriter, req *http.Request) {
	tokenString, ok := cookieValue(req, idTokenCookie)
	if !ok {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	user, err := h.tokens.Verify(tokenString)
	if err != nil {
		log.Printf("invalid token: %v", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"email": user.Email})
}
