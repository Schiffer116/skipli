package profile

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/auth"
)

const maxNameLength = 50

type Store struct {
	db    *dynamodb.Client
	table string
}

func NewStore(db *dynamodb.Client, table string) *Store {
	return &Store{db: db, table: table}
}

func key(userID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
		"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
	}
}

func DefaultName(email string) string {
	name, _, _ := strings.Cut(email, "@")
	return name
}

func (s *Store) Names(ctx context.Context, users []auth.User) (map[string]string, error) {
	names := map[string]string{}
	for _, u := range users {
		names[u.ID] = DefaultName(u.Email)
	}

	for start := 0; start < len(users); start += 100 {
		var keys []map[string]types.AttributeValue
		for _, u := range users[start:min(start+100, len(users))] {
			keys = append(keys, key(u.ID))
		}

		for len(keys) > 0 {
			out, err := s.db.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: map[string]types.KeysAndAttributes{s.table: {Keys: keys}},
			})
			if err != nil {
				return nil, err
			}

			var profiles []struct{ PK, Name string }
			if err := attributevalue.UnmarshalListOfMaps(out.Responses[s.table], &profiles); err != nil {
				return nil, err
			}
			for _, p := range profiles {
				if p.Name != "" {
					names[strings.TrimPrefix(p.PK, "USER#")] = p.Name
				}
			}

			keys = out.UnprocessedKeys[s.table].Keys
		}
	}
	return names, nil
}

type Me struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Handler struct {
	store  *Store
	tokens *auth.TokenVerifier
}

func NewHandler(store *Store, tokens *auth.TokenVerifier) *Handler {
	return &Handler{store: store, tokens: tokens}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /me", h.tokens.RequireAuth(h.get))
	router.HandleFunc("PUT /me", h.tokens.RequireAuth(h.put))
}

func (h *Handler) get(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	user, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "missing user in context", http.StatusInternalServerError)
		return
	}

	names, err := h.store.Names(ctx, []auth.User{user})
	if err != nil {
		log.Printf("failed to get profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Me{ID: user.ID, Email: user.Email, Name: names[user.ID]})
}

type PutRequest struct {
	Name string `json:"name"`
}

func (h *Handler) put(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	user, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "missing user in context", http.StatusInternalServerError)
		return
	}

	var r PutRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	name := strings.TrimSpace(r.Name)
	if name == "" || len([]rune(name)) > maxNameLength {
		http.Error(w, "name must be 1 to 50 characters", http.StatusBadRequest)
		return
	}

	item := key(user.ID)
	item["Name"] = &types.AttributeValueMemberS{Value: name}
	if _, err := h.store.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(h.store.table), Item: item}); err != nil {
		log.Printf("failed to save profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Me{ID: user.ID, Email: user.Email, Name: name})
}
