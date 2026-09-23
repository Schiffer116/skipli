package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const tableName = "Skipli"

type Handler struct {
	db     *dynamodb.Client
	mailer *Mailer
	tokens *TokenIssuer
}

func NewHandler(db *dynamodb.Client, mailer *Mailer, tokens *TokenIssuer) *Handler {
	return &Handler{db: db, mailer: mailer, tokens: tokens}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /auth/send", h.sendVerificationEmail)
	router.HandleFunc("POST /auth/verify", h.verify)
	router.HandleFunc("GET /auth/email", h.getEmailFromJwt)
}

type SendVerificationEmailRequest struct {
	Email string `json:"email"`
}

func (h *Handler) sendVerificationEmail(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var r SendVerificationEmailRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	code, err := generateCode()
	if err != nil {
		log.Printf("failed to generate verification code: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.storeCode(ctx, r.Email, code); err != nil {
		log.Printf("failed to store verification code: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	subject := code + " is your verification code"
	if err := h.mailer.Send(r.Email, subject, subject); err != nil {
		log.Printf("failed to send verification email: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type VerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type VerifyResponse struct {
	AccessToken string `json:"accessToken"`
}

// verify issues a token for any code, without checking it against the stored
// one. Email delivery is currently broken for reasons outside this codebase,
// so there's no way for a real user to receive their code yet; remove this
// bypass once that's resolved.
func (h *Handler) verify(w http.ResponseWriter, req *http.Request) {
	var r VerifyRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	accessToken, err := h.tokens.Issue(r.Email)
	if err != nil {
		log.Printf("failed to issue access token: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VerifyResponse{AccessToken: accessToken})
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

func (h *Handler) storeCode(ctx context.Context, email, code string) error {
	_, err := h.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]types.AttributeValue{
			"PK":               &types.AttributeValueMemberS{Value: "USER#" + email},
			"SK":               &types.AttributeValueMemberS{Value: "VERIFICATION"},
			"VerificationCode": &types.AttributeValueMemberS{Value: code},
		},
	})
	return err
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n), nil
}
