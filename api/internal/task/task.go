package task

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
	router.HandleFunc("GET /boards/{boardId}/cards/{cardId}/tasks", h.member(h.list))
	router.HandleFunc("GET /boards/{boardId}/cards/{cardId}/tasks/{taskId}", h.member(h.get))
}

// Task is the internal shape, including Order for sorting; Response is what
// actually goes over the wire. The old API's task responses leaked `order`
// via a naive object spread (`{...doc.data()}`) — that looks like an
// oversight, not a feature, so it's not replicated here; Order stays
// internal-only the same way it already does for cards.
type Task struct {
	ID          string  `json:"id"`
	CardID      string  `json:"cardId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Order       float64 `json:"-"`
}

type Response struct {
	ID          string `json:"id"`
	CardID      string `json:"cardId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func toResponse(t Task) Response {
	return Response{ID: t.ID, CardID: t.CardID, Name: t.Name, Description: t.Description, Status: t.Status}
}

func taskSK(cardID, taskID string) string {
	return "TASK#" + cardID + "#" + taskID
}

func taskPrefix(cardID string) string {
	return "TASK#" + cardID + "#"
}

// listTasks fetches every task on a card. Like cards, tasks have no index
// of their own — ordering happens in-app after a single Query.
func (h *Handler) listTasks(ctx context.Context, boardID, cardID string) ([]Task, error) {
	keyEx := expression.Key("PK").Equal(expression.Value(boardID)).
		And(expression.Key("SK").BeginsWith(taskPrefix(cardID)))
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

	tasks := []Task{}
	for queryOutput.HasMorePages() {
		page, err := queryOutput.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		var pageTasks []Task
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageTasks); err != nil {
			return nil, err
		}
		tasks = append(tasks, pageTasks...)
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Order < tasks[j].Order })
	return tasks, nil
}

func (h *Handler) getTask(ctx context.Context, boardID, cardID, taskID string) (Task, bool, error) {
	response, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: taskSK(cardID, taskID)},
		},
	})
	if err != nil {
		return Task{}, false, err
	}
	if response.Item == nil {
		return Task{}, false, nil
	}

	var task Task
	if err := attributevalue.UnmarshalMap(response.Item, &task); err != nil {
		return Task{}, false, err
	}
	return task, true, nil
}

func (h *Handler) list(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	tasks, err := h.listTasks(ctx, boardID, cardID)
	if err != nil {
		log.Printf("failed to list tasks: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]Response, len(tasks))
	for i, t := range tasks {
		response[i] = toResponse(t)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) get(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")
	taskID := req.PathValue("taskId")

	task, ok, err := h.getTask(ctx, boardID, cardID, taskID)
	if err != nil {
		log.Printf("failed to get task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, req)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toResponse(task))
}
