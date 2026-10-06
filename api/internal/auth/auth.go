package auth

import (
	"context"
	"crypto/rand"
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

// Handler signs users in with Cognito's emailed one-time codes.
type Handler struct {
	idp      *cip.Client
	poolID   string
	clientID string
	tokens   *TokenVerifier
}

func NewHandler(idp *cip.Client, poolID, clientID string, tokens *TokenVerifier) *Handler {
	return &Handler{idp: idp, poolID: poolID, clientID: clientID, tokens: tokens}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /auth/send", h.sendVerificationEmail)
	router.HandleFunc("POST /auth/verify", h.verify)
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

	if err := h.ensureUser(ctx, r.Email); err != nil {
		log.Printf("failed to create user: %v", err)
		http.Error(w, "failed to send verification email", http.StatusInternalServerError)
		return
	}

	// Cognito emails the code and returns a session to answer it with.
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

// canReceiveMail rejects malformed addresses and domains with no mail server,
// so typos like "@gmial.com" don't bounce. DNS outages fail open.
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
	// A single "." record is a null MX: the domain says it accepts no mail.
	return err == nil && len(mx) > 0 && mx[0].Host != "."
}

// ensureUser creates the Cognito user on first sign-in, confirmed and with a
// verified email so it can receive codes. The password is random and never
// used; it only moves the user out of FORCE_CHANGE_PASSWORD.
func (h *Handler) ensureUser(ctx context.Context, email string) error {
	_, err := h.idp.AdminCreateUser(ctx, &cip.AdminCreateUserInput{
		UserPoolId:    aws.String(h.poolID),
		Username:      aws.String(email),
		MessageAction: types.MessageActionTypeSuppress,
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
			{Name: aws.String("email_verified"), Value: aws.String("true")},
		},
	})
	var exists *types.UsernameExistsException
	if errors.As(err, &exists) {
		return nil
	}
	if err != nil {
		return err
	}

	_, err = h.idp.AdminSetUserPassword(ctx, &cip.AdminSetUserPasswordInput{
		UserPoolId: aws.String(h.poolID),
		Username:   aws.String(email),
		Password:   aws.String(rand.Text() + "Aa1!"), // meets the default password policy
		Permanent:  true,
	})
	return err
}

type VerifyRequest struct {
	Email   string `json:"email"`
	Code    string `json:"code"`
	Session string `json:"session"`
}

type VerifyResponse struct {
	AccessToken string `json:"accessToken"`
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

	// The ID token carries the email the rest of the API keys data on.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VerifyResponse{AccessToken: *out.AuthenticationResult.IdToken})
}

func (h *Handler) getEmailFromJwt(w http.ResponseWriter, req *http.Request) {
	tokenString, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !ok {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}

	email, err := h.tokens.Verify(tokenString)
	if err != nil {
		log.Printf("invalid token: %v", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"email": email})
}
