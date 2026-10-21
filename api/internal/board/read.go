package board

import (
	"cmp"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/Schiffer116/skipli/api/internal/auth"
)

// Whole is everything the app shows for an open board, in one read: the
// board, its members, and its cards in order, each with its tasks in order.
type Whole struct {
	ID          string   `json:"id"`
	Owner       string   `json:"owner"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []Member `json:"members"`
	Cards       []Card   `json:"cards"`
}

type Card struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Tasks       []Task  `json:"tasks"`
	Order       float64 `json:"-"`
}

type Task struct {
	ID          string  `json:"id"`
	CardID      string  `json:"cardId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Order       float64 `json:"-"`
}

// read reads a whole board. Its board item, members, cards and tasks all
// share the board's partition, so one Query (paged past 1 MB) finds them
// all; members' names then come from their profiles. ok is false when the
// board doesn't exist.
func (h *Handler) read(ctx context.Context, boardID string) (whole Whole, ok bool, err error) {
	expr, err := expression.NewBuilder().
		WithKeyCondition(expression.Key("PK").Equal(expression.Value(boardID))).
		Build()
	if err != nil {
		return Whole{}, false, err
	}
	pages := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})

	var users []auth.User
	cards := []Card{}
	var tasks []Task
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return Whole{}, false, err
		}
		for _, item := range page.Items {
			var key struct{ SK string }
			if err := attributevalue.UnmarshalMap(item, &key); err != nil {
				return Whole{}, false, err
			}
			switch {
			case key.SK == "META":
				ok = true
				var board Board
				err = attributevalue.UnmarshalMap(item, &board)
				whole.ID, whole.Owner, whole.Name, whole.Description = board.ID, board.Owner, board.Name, board.Description
			case strings.HasPrefix(key.SK, "MEMBER#"):
				var m struct{ Member, Email string }
				err = attributevalue.UnmarshalMap(item, &m)
				users = append(users, auth.User{ID: m.Member, Email: m.Email})
			case strings.HasPrefix(key.SK, "CARD#"):
				card := Card{Tasks: []Task{}}
				err = attributevalue.UnmarshalMap(item, &card)
				cards = append(cards, card)
			case strings.HasPrefix(key.SK, "TASK#"):
				var task Task
				err = attributevalue.UnmarshalMap(item, &task)
				tasks = append(tasks, task)
			}
			if err != nil {
				return Whole{}, false, err
			}
		}
	}
	if !ok {
		return Whole{}, false, nil
	}

	// Items come back in key order, so a stable sort keeps ties in a fixed
	// order too.
	slices.SortStableFunc(cards, func(a, b Card) int { return cmp.Compare(a.Order, b.Order) })
	slices.SortStableFunc(tasks, func(a, b Task) int { return cmp.Compare(a.Order, b.Order) })
	at := map[string]int{}
	for i, card := range cards {
		at[card.ID] = i
	}
	for _, task := range tasks {
		if i, ok := at[task.CardID]; ok {
			cards[i].Tasks = append(cards[i].Tasks, task)
		}
	}
	whole.Cards = cards

	names, err := h.profiles.Names(ctx, users)
	if err != nil {
		return Whole{}, false, err
	}
	whole.Members = []Member{}
	for _, u := range users {
		whole.Members = append(whole.Members, Member{ID: u.ID, Email: u.Email, Name: names[u.ID]})
	}
	return whole, true, nil
}

func (h *Handler) get(w http.ResponseWriter, req *http.Request) {
	whole, ok, err := h.read(req.Context(), req.PathValue("id"))
	if err != nil {
		log.Printf("failed to read board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, req)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(whole)
}
