// Package boardchange applies Board changes, the edits to the cards and
// tasks of one board, to the table. The realtime API takes them from a
// board's WebSocket and broadcasts the ones it accepted.
package boardchange

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/clientid"
)

const orderGap = 1000

// A Change's Type is one of these, matching the app's Change union in
// ui/src/offline/changes.ts.
const (
	CreateCard = "createCard"
	UpdateCard = "updateCard"
	DeleteCard = "deleteCard"
	MoveCard   = "moveCard"
	CreateTask = "createTask"
	UpdateTask = "updateTask"
	DeleteTask = "deleteTask"
	MoveTask   = "moveTask"
	CleanUp    = "cleanUp"
)

// Change is one Board change. Which fields it uses depends on its Type.
type Change struct {
	Type        string  `json:"type"`
	BoardID     string  `json:"boardId"`
	CardID      string  `json:"cardId"`
	TaskID      string  `json:"taskId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	NewCardID   string  `json:"newCardId"`
	BeforeID    *string `json:"beforeId"`
	AfterID     *string `json:"afterId"`
}

// MarshalJSON writes only the fields the change's Type uses, so teammates
// receive it in the same shape the app sent it.
func (c Change) MarshalJSON() ([]byte, error) {
	out := map[string]any{"type": c.Type, "boardId": c.BoardID}
	switch c.Type {
	case CreateCard:
		out["cardId"], out["name"], out["createdAt"] = c.CardID, c.Name, c.CreatedAt
	case UpdateCard:
		out["cardId"], out["name"], out["description"] = c.CardID, c.Name, c.Description
	case DeleteCard:
		out["cardId"] = c.CardID
	case MoveCard:
		out["cardId"], out["beforeId"], out["afterId"] = c.CardID, c.BeforeID, c.AfterID
	case CreateTask:
		out["cardId"], out["taskId"], out["name"] = c.CardID, c.TaskID, c.Name
	case UpdateTask:
		out["cardId"], out["taskId"] = c.CardID, c.TaskID
		out["name"], out["description"], out["status"] = c.Name, c.Description, c.Status
	case DeleteTask:
		out["cardId"], out["taskId"] = c.CardID, c.TaskID
	case MoveTask:
		out["cardId"], out["taskId"], out["newCardId"] = c.CardID, c.TaskID, c.NewCardID
		out["beforeId"], out["afterId"] = c.BeforeID, c.AfterID
	}
	return json.Marshal(out)
}

// Refusal is a change the board can't take, with the HTTP status that says
// why: 400 for a malformed change, 404 when what it changes is gone, 409 when
// a create's ID is already taken (the app treats that as already saved).
type Refusal struct {
	Status  int
	Message string
}

func (r *Refusal) Error() string { return r.Message }

func refuse(status int, message string) error {
	return &Refusal{Status: status, Message: message}
}

// Status is the HTTP status an Apply error stands for: a Refusal's own, or
// 500 for anything else.
func Status(err error) int {
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return refusal.Status
	}
	return http.StatusInternalServerError
}

type Store struct {
	db    *dynamodb.Client
	table string
}

func NewStore(db *dynamodb.Client, table string) *Store {
	return &Store{db: db, table: table}
}

// Apply writes a change to the board it names. The caller has already
// checked that whoever sent it may change that board.
func (s *Store) Apply(ctx context.Context, c Change) error {
	if err := c.validate(); err != nil {
		return err
	}
	switch c.Type {
	case CreateCard:
		return s.createCard(ctx, c)
	case UpdateCard:
		return s.updateCard(ctx, c)
	case DeleteCard:
		return s.deleteCard(ctx, c)
	case MoveCard:
		return s.moveCard(ctx, c)
	case CreateTask:
		return s.createTask(ctx, c)
	case UpdateTask:
		return s.updateTask(ctx, c)
	case DeleteTask:
		return s.deleteTask(ctx, c)
	case MoveTask:
		return s.moveTask(ctx, c)
	case CleanUp:
		return s.cleanUp(ctx, c)
	}
	return refuse(http.StatusBadRequest, "unknown change type")
}

func (c Change) validate() error {
	if c.BoardID == "" {
		return refuse(http.StatusBadRequest, "missing boardId")
	}
	needCard := c.Type != CleanUp
	needTask := c.Type == CreateTask || c.Type == UpdateTask || c.Type == DeleteTask || c.Type == MoveTask
	if needCard && c.CardID == "" || needTask && c.TaskID == "" || c.Type == MoveTask && c.NewCardID == "" {
		return refuse(http.StatusBadRequest, "missing id")
	}
	// The app names what it creates, so a retried create can't make a
	// duplicate; only canonical UUIDs can become keys.
	created := map[string]string{CreateCard: c.CardID, CreateTask: c.TaskID}
	if id, ok := created[c.Type]; ok {
		if _, err := clientid.Pick(id); err != nil {
			return refuse(http.StatusBadRequest, err.Error())
		}
	}
	return nil
}

func isConditionFailed(err error) bool {
	var failed *types.ConditionalCheckFailedException
	return errors.As(err, &failed)
}

// canceledBy reports whether a transaction was canceled by the condition of
// its i-th item.
func canceledBy(err error, i int) bool {
	var canceled *types.TransactionCanceledException
	return errors.As(err, &canceled) && i < len(canceled.CancellationReasons) &&
		canceled.CancellationReasons[i].Code != nil && *canceled.CancellationReasons[i].Code == "ConditionalCheckFailed"
}
