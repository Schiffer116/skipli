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
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
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

func (h *Handler) verify(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var r VerifyRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	storedCode, ok, err := h.getStoredCode(ctx, r.Email)
	if err != nil {
		log.Printf("failed to get verification code: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok || r.Code == "" || r.Code != storedCode {
		http.Error(w, "invalid email or verification code", http.StatusUnauthorized)
		return
	}

	// One-time use, matching the old API deleting the record on success.
	if err := h.deleteCode(ctx, r.Email); err != nil {
		log.Printf("failed to delete verification code: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

func (h *Handler) getStoredCode(ctx context.Context, email string) (string, bool, error) {
	response, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + email},
			"SK": &types.AttributeValueMemberS{Value: "VERIFICATION"},
		},
	})
	if err != nil {
		return "", false, err
	}
	if response.Item == nil {
		return "", false, nil
	}

	var item struct{ VerificationCode string }
	if err := attributevalue.UnmarshalMap(response.Item, &item); err != nil {
		return "", false, err
	}
	return item.VerificationCode, true, nil
}

func (h *Handler) deleteCode(ctx context.Context, email string) error {
	_, err := h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + email},
			"SK": &types.AttributeValueMemberS{Value: "VERIFICATION"},
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
