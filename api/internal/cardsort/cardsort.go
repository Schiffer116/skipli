// Package cardsort serves Sort: the order a criterion someone types, such as
// "most urgent first", puts a card's tasks in, as Jev judges it. It changes
// nothing; the app makes the moves as ordinary Board changes.
package cardsort

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/access"
	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/jev"
)

const (
	maxCriterion = 200
	maxTasks     = 200
)

type Handler struct {
	db     *dynamodb.Client
	table  string
	tokens *auth.TokenVerifier
	judge  jev.Judge
	limit  *Limit
}

func NewHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier, judge jev.Judge) *Handler {
	return &Handler{db: db, table: table, tokens: tokens, judge: judge, limit: NewLimit(db, table)}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("POST /boards/{boardId}/cards/{cardId}/sort",
		board.RequireMember(h.db, h.table, h.tokens, "boardId", h.sort))
}

type Request struct {
	Criterion string `json:"criterion"`
}

type Response struct {
	// Order is the card's task IDs, as the criterion puts them.
	Order []string `json:"order"`
	// Model is the Jev release that judged, or "fake".
	Model string `json:"model"`
}

func (h *Handler) sort(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID, cardID := req.PathValue("boardId"), req.PathValue("cardId")

	var body Request
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	criterion := strings.TrimSpace(body.Criterion)
	if criterion == "" || utf8.RuneCountInString(criterion) > maxCriterion {
		http.Error(w, "criterion must be 1 to 200 characters", http.StatusBadRequest)
		return
	}

	tasks, ok, err := h.tasks(ctx, boardID, cardID)
	if err != nil {
		log.Printf("failed to read card %s: %v", cardID, err)
		http.Error(w, "failed to read card", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "card not found", http.StatusNotFound)
		return
	}

	order := make([]string, len(tasks))
	for i, task := range tasks {
		order[i] = task.ID
	}
	if len(tasks) < 2 {
		writeJSON(w, Response{Order: order})
		return
	}
	if len(tasks) > maxTasks || jev.TooLarge(criterion, tasks) {
		http.Error(w, jev.ErrTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}

	user, _ := auth.UserFromContext(ctx)
	switch allowed, wait, err := h.limit.Take(ctx, user.ID, time.Now()); {
	case err != nil:
		log.Printf("failed to count sort: %v", err)
		http.Error(w, "failed to sort", http.StatusInternalServerError)
		return
	case !allowed:
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second).Seconds())))
		http.Error(w, "sorting too often, try again shortly", http.StatusTooManyRequests)
		return
	}

	order, model, err := jev.Rank(ctx, h.judge, criterion, tasks)
	switch {
	case errors.Is(err, jev.ErrNotConfigured):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, jev.ErrUnavailable):
		log.Printf("failed to sort card %s: %v", cardID, err)
		http.Error(w, "Jev is unavailable, try again", http.StatusServiceUnavailable)
	case errors.Is(err, jev.ErrTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case err != nil:
		log.Printf("failed to sort card %s: %v", cardID, err)
		http.Error(w, "failed to sort", http.StatusInternalServerError)
	default:
		writeJSON(w, Response{Order: order, Model: model})
	}
}

// tasks reads a card's tasks in their order. ok is false when the card
// doesn't exist.
func (h *Handler) tasks(ctx context.Context, boardID, cardID string) ([]jev.Task, bool, error) {
	ok, err := access.Exists(ctx, h.db, h.table, boardID, "CARD#"+cardID)
	if err != nil || !ok {
		return nil, ok, err
	}

	expr, err := expression.NewBuilder().WithKeyCondition(
		expression.Key("PK").Equal(expression.Value(boardID)).
			And(expression.Key("SK").BeginsWith("TASK#" + cardID + "#")),
	).Build()
	if err != nil {
		return nil, false, err
	}
	pages := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})
	var items []board.Task
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, false, err
		}
		var tasks []board.Task
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &tasks); err != nil {
			return nil, false, err
		}
		items = append(items, tasks...)
	}
	// Items come back in key order, so ties keep a fixed order, as when
	// the board is read.
	slices.SortStableFunc(items, func(a, b board.Task) int { return cmp.Compare(a.Order, b.Order) })

	tasks := make([]jev.Task, len(items))
	for i, t := range items {
		tasks[i] = jev.Task{ID: t.ID, Name: t.Name, Description: t.Description, Status: t.Status}
	}
	return tasks, true, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
