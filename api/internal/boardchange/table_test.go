package boardchange

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

type tableTask struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type tableCard struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Tasks       []tableTask `json:"tasks"`
}

// TestBoardChangesTable applies every case of testdata/board-changes.json,
// which the app's reducer passes too, to a table and reads the board back.
func TestBoardChangesTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/board-changes.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Board []tableCard `json:"board"`
		Cases []struct {
			Name   string          `json:"name"`
			Before []tableCard     `json:"before"`
			Change json.RawMessage `json:"change"`
			After  json.RawMessage `json:"after"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	for _, tc := range doc.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			before := tc.Before
			if before == nil {
				before = doc.Board
			}
			table := awsfake.NewTable()
			seed(table, before)
			fake := awsfake.New(t, table.Respond)

			var c Change
			if err := json.Unmarshal(tc.Change, &c); err != nil {
				t.Fatal(err)
			}
			c.BoardID = "b1"
			err := NewStore(fake.DynamoDB(), "Skipli").Apply(context.Background(), c)
			// A create the board already has was saved by an earlier attempt
			// (alreadySaved in ui/src/offline/changes.ts).
			if Status(err) == http.StatusConflict && strings.HasPrefix(c.Type, "create") {
				err = nil
			}

			got := readBack(table)
			if string(tc.After) == `"dropped"` {
				if err == nil || Status(err) >= 500 {
					t.Fatalf("got %v, want it refused (dropped)", err)
				}
				if !reflect.DeepEqual(got, before) {
					t.Errorf("a dropped change still changed the board:\n%+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("got %v, want it applied", err)
			}
			var want []tableCard
			if err := json.Unmarshal(tc.After, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func attrS(value string) map[string]any { return map[string]any{"S": value} }

func attrN(order int) map[string]any { return map[string]any{"N": strconv.Itoa(order)} }

func seed(table *awsfake.Table, cards []tableCard) {
	table.Put(map[string]any{"PK": attrS("b1"), "SK": attrS("META"), "Name": attrS("Plans")})
	for i, card := range cards {
		table.Put(map[string]any{
			"PK": attrS("b1"), "SK": attrS(cardSK(card.ID)), "ID": attrS(card.ID), "Name": attrS(card.Name),
			"Description": attrS(card.Description), "Order": attrN((i + 1) * orderGap),
		})
		for j, task := range card.Tasks {
			table.Put(map[string]any{
				"PK": attrS("b1"), "SK": attrS(taskSK(card.ID, task.ID)), "ID": attrS(task.ID), "CardID": attrS(card.ID),
				"Name": attrS(task.Name), "Description": attrS(task.Description), "Status": attrS(task.Status),
				"Order": attrN((j + 1) * orderGap),
			})
		}
	}
}

// readBack reads the board as internal/board's read does: items in key
// order, then stably by order.
func readBack(table *awsfake.Table) []tableCard {
	type placed struct {
		order float64
		card  string
		item  map[string]any
	}
	var cards, tasks []placed
	for _, item := range table.Items() {
		sk := item["SK"].(map[string]any)["S"].(string)
		var order float64
		if o, ok := item["Order"].(map[string]any); ok {
			order, _ = strconv.ParseFloat(o["N"].(string), 64)
		}
		switch {
		case strings.HasPrefix(sk, "CARD#"):
			cards = append(cards, placed{order: order, item: item})
		case strings.HasPrefix(sk, "TASK#"):
			tasks = append(tasks, placed{order: order, card: item["CardID"].(map[string]any)["S"].(string), item: item})
		}
	}
	byOrder := func(a, b placed) int { return cmp.Compare(a.order, b.order) }
	slices.SortStableFunc(cards, byOrder)
	slices.SortStableFunc(tasks, byOrder)

	field := func(item map[string]any, name string) string { return item[name].(map[string]any)["S"].(string) }
	board := []tableCard{}
	for _, card := range cards {
		c := tableCard{ID: field(card.item, "ID"), Name: field(card.item, "Name"), Description: field(card.item, "Description"), Tasks: []tableTask{}}
		for _, task := range tasks {
			if task.card == c.ID {
				c.Tasks = append(c.Tasks, tableTask{
					ID: field(task.item, "ID"), Name: field(task.item, "Name"),
					Description: field(task.item, "Description"), Status: field(task.item, "Status"),
				})
			}
		}
		board = append(board, c)
	}
	return board
}
