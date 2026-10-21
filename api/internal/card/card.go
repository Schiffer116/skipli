package card

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
)

type Handler struct {
	db     *dynamodb.Client
	table  string
	tokens *auth.TokenVerifier
}

func NewHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier) *Handler {
	return &Handler{db: db, table: table, tokens: tokens}
}

func (h *Handler) member(next http.HandlerFunc) http.HandlerFunc {
	return board.RequireMember(h.db, h.table, h.tokens, "boardId", next)
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /boards/{boardId}/cards", h.member(h.list))
	router.HandleFunc("GET /boards/{boardId}/cards/{cardId}", h.member(h.get))
}

// Card is the internal shape, including Order for sorting; Response is what
// actually goes over the wire — Order is deliberately never exposed, same as
// the old API never returned it either.
type Card struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Order       float64 `json:"-"`
}

type Response struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func toResponse(c Card) Response {
	return Response{ID: c.ID, Name: c.Name, Description: c.Description}
}

func cardSK(cardID string) string {
	return "CARD#" + cardID
}

// listCards fetches every card on a board. Cards have no index of their
// own, so ordering happens in-app after a single Query rather than via a
// second GSI — fine at the scale a single board's card count stays at.
func (h *Handler) listCards(ctx context.Context, boardID string) ([]Card, error) {
	keyEx := expression.Key("PK").Equal(expression.Value(boardID)).
		And(expression.Key("SK").BeginsWith("CARD#"))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, err
	}

	queryOutput := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})

	cards := []Card{}
	for queryOutput.HasMorePages() {
		page, err := queryOutput.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		var pageCards []Card
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageCards); err != nil {
			return nil, err
		}
		cards = append(cards, pageCards...)
	}

	sort.Slice(cards, func(i, j int) bool { return cards[i].Order < cards[j].Order })
	return cards, nil
}

func (h *Handler) getCard(ctx context.Context, boardID, cardID string) (Card, bool, error) {
	response, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: cardSK(cardID)},
		},
	})
	if err != nil {
		return Card{}, false, err
	}
	if response.Item == nil {
		return Card{}, false, nil
	}

	var card Card
	if err := attributevalue.UnmarshalMap(response.Item, &card); err != nil {
		return Card{}, false, err
	}
	return card, true, nil
}

func (h *Handler) list(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")

	cards, err := h.listCards(ctx, boardID)
	if err != nil {
		log.Printf("failed to list cards: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]Response, len(cards))
	for i, c := range cards {
		response[i] = toResponse(c)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) get(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	card, ok, err := h.getCard(ctx, boardID, cardID)
	if err != nil {
		log.Printf("failed to get card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, req)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toResponse(card))
}
