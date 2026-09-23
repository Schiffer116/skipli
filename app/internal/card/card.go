package card

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const tableName = "Skipli"
const orderGap = 1000

// taskPrefix mirrors internal/task's own SK format ("TASK#<cardId>#..."),
// duplicated here (not imported) so cascade-delete can find a card's tasks
// without coupling the two packages — same duplication-over-sharing choice
// already made for tableName across this codebase's packages.
func taskPrefix(cardID string) string {
	return "TASK#" + cardID + "#"
}

type Handler struct {
	db *dynamodb.Client
}

func NewHandler(db *dynamodb.Client) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /boards/{boardId}/cards", h.list)
	router.HandleFunc("POST /boards/{boardId}/cards", h.create)
	router.HandleFunc("GET /boards/{boardId}/cards/{cardId}", h.get)
	router.HandleFunc("PUT /boards/{boardId}/cards/{cardId}", h.update)
	router.HandleFunc("PATCH /boards/{boardId}/cards/{cardId}", h.reorder)
	router.HandleFunc("DELETE /boards/{boardId}/cards/{cardId}", h.delete)
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
		TableName:                 aws.String(tableName),
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
		TableName: aws.String(tableName),
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

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

func (h *Handler) create(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")

	var r CreateRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	cards, err := h.listCards(ctx, boardID)
	if err != nil {
		log.Printf("failed to list cards: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	order := float64(orderGap)
	if len(cards) > 0 {
		order = cards[len(cards)-1].Order + orderGap
	}

	id := uuid.New().String()
	_, err = h.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]types.AttributeValue{
			"PK":          &types.AttributeValueMemberS{Value: boardID},
			"SK":          &types.AttributeValueMemberS{Value: cardSK(id)},
			"ID":          &types.AttributeValueMemberS{Value: id},
			"Name":        &types.AttributeValueMemberS{Value: r.Name},
			"Description": &types.AttributeValueMemberS{Value: r.Description},
			"CreatedAt":   &types.AttributeValueMemberS{Value: r.CreatedAt},
			"Order":       &types.AttributeValueMemberN{Value: strconv.FormatFloat(order, 'f', -1, 64)},
		},
	})
	if err != nil {
		log.Printf("failed to create card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(Response{ID: id, Name: r.Name, Description: r.Description})
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

type UpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) update(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	var r UpdateRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	update := expression.Set(expression.Name("Name"), expression.Value(r.Name)).
		Set(expression.Name("Description"), expression.Value(r.Description))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		log.Printf("failed to build update expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: cardSK(cardID)},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Printf("failed to update card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{ID: cardID, Name: r.Name, Description: r.Description})
}

type ReorderRequest struct {
	BeforeID string `json:"beforeId"`
	AfterID  string `json:"afterId"`
}

func (h *Handler) reorder(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	var r ReorderRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	if r.BeforeID == "" && r.AfterID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var beforeCard, afterCard *Card
	if r.BeforeID != "" {
		c, ok, err := h.getCard(ctx, boardID, r.BeforeID)
		if err != nil {
			log.Printf("failed to get before card: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, req)
			return
		}
		beforeCard = &c
	}
	if r.AfterID != "" {
		c, ok, err := h.getCard(ctx, boardID, r.AfterID)
		if err != nil {
			log.Printf("failed to get after card: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, req)
			return
		}
		afterCard = &c
	}

	var newOrder float64
	switch {
	case beforeCard == nil:
		newOrder = afterCard.Order - orderGap
	case afterCard == nil:
		newOrder = beforeCard.Order + orderGap
	default:
		newOrder = beforeCard.Order + (afterCard.Order-beforeCard.Order)/2
	}

	update := expression.Set(expression.Name("Order"), expression.Value(newOrder))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		log.Printf("failed to build update expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: cardSK(cardID)},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Printf("failed to reorder card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// delete cascades to the card's tasks, all in one transaction so a card
// can't be left half-deleted (tasks gone but card still there, or vice
// versa). Caps out at DynamoDB's 100-items-per-transaction limit, same
// unhandled-at-scale caveat as list's BatchGetItem elsewhere in this repo.
func (h *Handler) delete(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	keyEx := expression.Key("PK").Equal(expression.Value(boardID)).
		And(expression.Key("SK").BeginsWith(taskPrefix(cardID)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	queryOutput, err := h.db.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      aws.String("SK"),
	})
	if err != nil {
		log.Printf("failed to list tasks for card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	transactItems := make([]types.TransactWriteItem, 0, len(queryOutput.Items)+1)
	for _, item := range queryOutput.Items {
		transactItems = append(transactItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: boardID},
					"SK": item["SK"],
				},
			},
		})
	}
	transactItems = append(transactItems, types.TransactWriteItem{
		Delete: &types.Delete{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: boardID},
				"SK": &types.AttributeValueMemberS{Value: cardSK(cardID)},
			},
		},
	})

	_, err = h.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: transactItems})
	if err != nil {
		log.Printf("failed to delete card: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
