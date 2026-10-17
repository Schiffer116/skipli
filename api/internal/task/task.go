package task

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/clientid"
)

const orderGap = 1000

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
	router.HandleFunc("POST /boards/{boardId}/cards/{cardId}/tasks", h.member(h.create))
	router.HandleFunc("GET /boards/{boardId}/cards/{cardId}/tasks/{taskId}", h.member(h.get))
	router.HandleFunc("PUT /boards/{boardId}/cards/{cardId}/tasks/{taskId}", h.member(h.update))
	router.HandleFunc("PATCH /boards/{boardId}/cards/{cardId}/tasks/{taskId}", h.member(h.move))
	router.HandleFunc("DELETE /boards/{boardId}/cards/{cardId}/tasks/{taskId}", h.member(h.delete))
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

// cardSK mirrors internal/card's SK format, duplicated (not imported) like
// card's taskPrefix, so creating a task can check its card exists without
// coupling the two packages.
func cardSK(cardID string) string {
	return "CARD#" + cardID
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

type CreateRequest struct {
	// ID is optional: the app names tasks it creates offline.
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *Handler) create(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")

	var r CreateRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	tasks, err := h.listTasks(ctx, boardID, cardID)
	if err != nil {
		log.Printf("failed to list tasks: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	order := float64(orderGap)
	if len(tasks) > 0 {
		order = tasks[len(tasks)-1].Order + orderGap
	}

	id, err := clientid.Pick(r.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The card must still exist, or a create sent after the card was
	// deleted (such as an offline change sent late) would leave a task
	// that no card shows.
	_, err = h.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				ConditionCheck: &types.ConditionCheck{
					TableName: aws.String(h.table),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: boardID},
						"SK": &types.AttributeValueMemberS{Value: cardSK(cardID)},
					},
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(h.table),
					Item: map[string]types.AttributeValue{
						"PK":          &types.AttributeValueMemberS{Value: boardID},
						"SK":          &types.AttributeValueMemberS{Value: taskSK(cardID, id)},
						"ID":          &types.AttributeValueMemberS{Value: id},
						"CardID":      &types.AttributeValueMemberS{Value: cardID},
						"Name":        &types.AttributeValueMemberS{Value: r.Name},
						"Description": &types.AttributeValueMemberS{Value: ""},
						"Status":      &types.AttributeValueMemberS{Value: "pending"},
						"Order":       &types.AttributeValueMemberN{Value: strconv.FormatFloat(order, 'f', -1, 64)},
					},
					ConditionExpression: aws.String("attribute_not_exists(SK)"),
				},
			},
		},
	})
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) && len(canceled.CancellationReasons) == 2 {
		if aws.ToString(canceled.CancellationReasons[0].Code) == "ConditionalCheckFailed" {
			http.Error(w, "card not found", http.StatusNotFound)
			return
		}
		if aws.ToString(canceled.CancellationReasons[1].Code) == "ConditionalCheckFailed" {
			http.Error(w, "a task with this id already exists", http.StatusConflict)
			return
		}
	}
	if err != nil {
		log.Printf("failed to create task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(Response{ID: id, CardID: cardID, Name: r.Name, Description: "", Status: "pending"})
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

type UpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (h *Handler) update(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")
	taskID := req.PathValue("taskId")

	var r UpdateRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	update := expression.Set(expression.Name("Name"), expression.Value(r.Name)).
		Set(expression.Name("Description"), expression.Value(r.Description)).
		Set(expression.Name("Status"), expression.Value(r.Status))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		log.Printf("failed to build update expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: taskSK(cardID, taskID)},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		// Without it, updating a deleted task would bring back a broken one.
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	if isConditionFailed(err) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("failed to update task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{ID: taskID, CardID: cardID, Name: r.Name, Description: r.Description, Status: r.Status})
}

type MoveRequest struct {
	NewCardID string `json:"newCardId"`
	BeforeID  string `json:"beforeId"`
	AfterID   string `json:"afterId"`
}

// move relocates a task to (possibly) a different card, recomputing its
// Order among the destination card's tasks — mirrors the old API, which had
// no early-return for "nothing specified" the way card reorder does, so a
// move with neither beforeId nor afterId still happens (landing at Order 0).
func (h *Handler) move(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")
	taskID := req.PathValue("taskId")

	var r MoveRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	task, ok, err := h.getTask(ctx, boardID, cardID, taskID)
	if err != nil {
		log.Printf("failed to get task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task doesn't exist", http.StatusNotFound)
		return
	}

	var beforeTask, afterTask *Task
	if r.BeforeID != "" {
		t, ok, err := h.getTask(ctx, boardID, r.NewCardID, r.BeforeID)
		if err != nil {
			log.Printf("failed to get before task: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, req)
			return
		}
		beforeTask = &t
	}
	if r.AfterID != "" {
		t, ok, err := h.getTask(ctx, boardID, r.NewCardID, r.AfterID)
		if err != nil {
			log.Printf("failed to get after task: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, req)
			return
		}
		afterTask = &t
	}

	var newOrder float64
	switch {
	case beforeTask == nil && afterTask == nil:
		newOrder = 0
	case beforeTask == nil:
		newOrder = afterTask.Order - orderGap
	case afterTask == nil:
		newOrder = beforeTask.Order + orderGap
	default:
		newOrder = beforeTask.Order + (afterTask.Order-beforeTask.Order)/2
	}

	if r.NewCardID == cardID {
		// Same card — just update Order in place. DynamoDB's
		// TransactWriteItems rejects two operations against the same key in
		// one call, so the delete+put path below can't be used when the SK
		// (which embeds cardId) isn't actually changing.
		update := expression.Set(expression.Name("Order"), expression.Value(newOrder))
		expr, err := expression.NewBuilder().WithUpdate(update).Build()
		if err != nil {
			log.Printf("failed to build update expression: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(h.table),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: boardID},
				"SK": &types.AttributeValueMemberS{Value: taskSK(cardID, taskID)},
			},
			UpdateExpression:          expr.Update(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ConditionExpression:       aws.String("attribute_exists(SK)"),
		})
		if isConditionFailed(err) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("failed to reorder task: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	// Different card — the SK embeds cardId, so "moving" means delete + put,
	// done as one transaction so a task is never dropped if the second step
	// fails. Both the task and the card it goes to must still be there, or
	// the move would bring back a deleted task or leave it in a deleted card.
	_, err = h.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Delete: &types.Delete{
					TableName: aws.String(h.table),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: boardID},
						"SK": &types.AttributeValueMemberS{Value: taskSK(cardID, taskID)},
					},
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				ConditionCheck: &types.ConditionCheck{
					TableName: aws.String(h.table),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: boardID},
						"SK": &types.AttributeValueMemberS{Value: cardSK(r.NewCardID)},
					},
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(h.table),
					Item: map[string]types.AttributeValue{
						"PK":          &types.AttributeValueMemberS{Value: boardID},
						"SK":          &types.AttributeValueMemberS{Value: taskSK(r.NewCardID, taskID)},
						"ID":          &types.AttributeValueMemberS{Value: taskID},
						"CardID":      &types.AttributeValueMemberS{Value: r.NewCardID},
						"Name":        &types.AttributeValueMemberS{Value: task.Name},
						"Description": &types.AttributeValueMemberS{Value: task.Description},
						"Status":      &types.AttributeValueMemberS{Value: task.Status},
						"Order":       &types.AttributeValueMemberN{Value: strconv.FormatFloat(newOrder, 'f', -1, 64)},
					},
				},
			},
		},
	})
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) && len(canceled.CancellationReasons) == 3 {
		if aws.ToString(canceled.CancellationReasons[0].Code) == "ConditionalCheckFailed" {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		if aws.ToString(canceled.CancellationReasons[1].Code) == "ConditionalCheckFailed" {
			http.Error(w, "card not found", http.StatusNotFound)
			return
		}
	}
	if err != nil {
		log.Printf("failed to move task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) delete(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	boardID := req.PathValue("boardId")
	cardID := req.PathValue("cardId")
	taskID := req.PathValue("taskId")

	_, err := h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: taskSK(cardID, taskID)},
		},
	})
	if err != nil {
		log.Printf("failed to delete task: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// isConditionFailed reports whether a write was refused by its
// ConditionExpression.
func isConditionFailed(err error) bool {
	var failed *types.ConditionalCheckFailedException
	return errors.As(err, &failed)
}
