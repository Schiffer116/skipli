package boardchange

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/board"
)

// The contract: Reduce, which edits a board file, and Store.Apply, which
// edits the table, answer every change the same way, with the same refusal
// or the same resulting board. memTable is a DynamoDB that remembers just
// enough of the operations the Store uses.

type memTable struct {
	mu    sync.Mutex
	items map[string]map[string]any // "PK/SK" to the item, attribute values as JSON
}

func (m *memTable) key(item map[string]any) string {
	return str(item["PK"]) + "/" + str(item["SK"])
}

func str(av any) string {
	s, _ := av.(map[string]any)["S"].(string)
	return s
}

func num(av any) float64 {
	n, _ := av.(map[string]any)["N"].(string)
	f, _ := strconv.ParseFloat(n, 64)
	return f
}

func s(v string) map[string]any { return map[string]any{"S": v} }
func n(v float64) map[string]any {
	return map[string]any{"N": strconv.FormatFloat(v, 'f', -1, 64)}
}

// holds checks a condition expression on the item at key.
func (m *memTable) holds(condition any, key string) bool {
	switch condition {
	case nil:
		return true
	case "attribute_exists(SK)":
		return m.items[key] != nil
	case "attribute_not_exists(SK)":
		return m.items[key] == nil
	}
	panic(fmt.Sprintf("memTable: condition %v", condition))
}

func (m *memTable) update(body map[string]any) {
	key := str(body["Key"].(map[string]any)["PK"]) + "/" + str(body["Key"].(map[string]any)["SK"])
	names := body["ExpressionAttributeNames"].(map[string]any)
	values := body["ExpressionAttributeValues"].(map[string]any)
	set, ok := strings.CutPrefix(strings.TrimSpace(body["UpdateExpression"].(string)), "SET ")
	if !ok {
		panic("memTable: update " + body["UpdateExpression"].(string))
	}
	for _, assignment := range strings.Split(set, ",") {
		lhs, rhs, _ := strings.Cut(strings.TrimSpace(assignment), " = ")
		m.items[key][names[lhs].(string)] = values[rhs]
	}
}

func canceledReasons(reasons ...string) (int, any) {
	codes := []any{}
	for _, reason := range reasons {
		codes = append(codes, map[string]any{"Code": reason})
	}
	return 400, map[string]any{"__type": "TransactionCanceledException", "Message": "canceled", "CancellationReasons": codes}
}

func (m *memTable) respond(op string, body map[string]any) (int, any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch op {
	case "GetItem":
		k := body["Key"].(map[string]any)
		if item := m.items[str(k["PK"])+"/"+str(k["SK"])]; item != nil {
			return 200, map[string]any{"Item": item}
		}
		return 200, map[string]any{}
	case "PutItem":
		item := body["Item"].(map[string]any)
		if !m.holds(body["ConditionExpression"], m.key(item)) {
			return 400, conditionFailed
		}
		m.items[m.key(item)] = item
		return 200, map[string]any{}
	case "UpdateItem":
		k := body["Key"].(map[string]any)
		if !m.holds(body["ConditionExpression"], str(k["PK"])+"/"+str(k["SK"])) {
			return 400, conditionFailed
		}
		m.update(body)
		return 200, map[string]any{}
	case "DeleteItem":
		k := body["Key"].(map[string]any)
		delete(m.items, str(k["PK"])+"/"+str(k["SK"]))
		return 200, map[string]any{}
	case "Query":
		values := body["ExpressionAttributeValues"].(map[string]any)
		// The values are the board, a key prefix and, with a filter, the id
		// the item must have.
		var pk, prefix, id string
		for _, v := range values {
			val := str(v)
			isBoard := false
			for key := range m.items {
				isBoard = isBoard || strings.HasPrefix(key, val+"/")
			}
			switch {
			case strings.HasPrefix(val, "CARD#"), strings.HasPrefix(val, "TASK#"):
				prefix = val
			case isBoard:
				pk = val
			default:
				id = val
			}
		}
		var keys []string
		for key, item := range m.items {
			if strings.HasPrefix(key, pk+"/"+prefix) && (body["FilterExpression"] == nil || str(item["ID"]) == id) {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		items := []any{}
		for _, key := range keys {
			items = append(items, m.items[key])
		}
		return 200, map[string]any{"Items": items}
	case "BatchWriteItem":
		for _, requests := range body["RequestItems"].(map[string]any) {
			for _, r := range requests.([]any) {
				k := r.(map[string]any)["DeleteRequest"].(map[string]any)["Key"].(map[string]any)
				delete(m.items, str(k["PK"])+"/"+str(k["SK"]))
			}
		}
		return 200, map[string]any{"UnprocessedItems": map[string]any{}}
	case "TransactWriteItems":
		return m.transact(body["TransactItems"].([]any))
	}
	panic("memTable: " + op)
}

func (m *memTable) transact(txItems []any) (int, any) {
	failed, reasons := false, []string{}
	for _, it := range txItems {
		it := it.(map[string]any)
		reason := "None"
		for _, kind := range []string{"ConditionCheck", "Put", "Delete"} {
			op, ok := it[kind].(map[string]any)
			if !ok {
				continue
			}
			var key string
			if kind == "Put" {
				key = m.key(op["Item"].(map[string]any))
			} else {
				k := op["Key"].(map[string]any)
				key = str(k["PK"]) + "/" + str(k["SK"])
			}
			if !m.holds(op["ConditionExpression"], key) {
				reason, failed = "ConditionalCheckFailed", true
			}
		}
		reasons = append(reasons, reason)
	}
	if failed {
		return canceledReasons(reasons...)
	}
	for _, it := range txItems {
		it := it.(map[string]any)
		if put, ok := it["Put"].(map[string]any); ok {
			m.items[m.key(put["Item"].(map[string]any))] = put["Item"].(map[string]any)
		}
		if del, ok := it["Delete"].(map[string]any); ok {
			k := del["Key"].(map[string]any)
			delete(m.items, str(k["PK"])+"/"+str(k["SK"]))
		}
	}
	return 200, map[string]any{}
}

// snapshot reads the table back the way the app's board read does.
func (m *memTable) snapshot(boardID string) board.Whole {
	var cards []board.Card
	var tasks []board.Task
	keys := make([]string, 0, len(m.items))
	for key := range m.items {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		item := m.items[key]
		switch sk := str(item["SK"]); {
		case strings.HasPrefix(sk, "CARD#"):
			cards = append(cards, board.Card{ID: str(item["ID"]), Name: str(item["Name"]), Description: str(item["Description"]), Order: num(item["Order"]), Tasks: []board.Task{}})
		case strings.HasPrefix(sk, "TASK#"):
			tasks = append(tasks, board.Task{ID: str(item["ID"]), CardID: str(item["CardID"]), Name: str(item["Name"]), Description: str(item["Description"]), Status: str(item["Status"]), Order: num(item["Order"])})
		}
	}
	slices.SortStableFunc(cards, func(a, b board.Card) int { return cmp.Compare(a.Order, b.Order) })
	slices.SortStableFunc(tasks, func(a, b board.Task) int { return cmp.Compare(a.Order, b.Order) })
	for _, task := range tasks {
		task.Order = 0
		for i := range cards {
			if cards[i].ID == task.CardID {
				cards[i].Tasks = append(cards[i].Tasks, task)
			}
		}
	}
	for i := range cards {
		cards[i].Order = 0
	}
	return board.Whole{ID: boardID, Name: "Roadmap", Description: "plans", Cards: cards}
}

func uid(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", i) }

// ids of the seed board and of what the cases create.
var (
	b1                 = uid(1)
	c1, c2, c3         = uid(11), uid(12), uid(13) // c3 has no tasks
	t1, t2, t3         = uid(21), uid(22), uid(23) // t1, t2 in c1; t3 in c2
	newCard, newTask   = uid(31), uid(32)
	missing, missingID = uid(98), uid(99)
)

// seed is the board every case starts from: three cards, three tasks.
func seedBoard() board.Whole {
	task := func(id, card, name, desc, status string) board.Task {
		return board.Task{ID: id, CardID: card, Name: name, Description: desc, Status: status}
	}
	return board.Whole{ID: b1, Name: "Roadmap", Description: "plans", Cards: []board.Card{
		{ID: c1, Name: "Backlog", Description: "later", Tasks: []board.Task{task(t1, c1, "Write", "all", "pending"), task(t2, c1, "Ship", "", "done")}},
		{ID: c2, Name: "Doing", Tasks: []board.Task{task(t3, c2, "Review", "", "pending")}},
		{ID: c3, Name: "Done", Tasks: []board.Task{}},
	}}
}

func (m *memTable) load(w board.Whole) {
	m.items = map[string]map[string]any{}
	put := func(item map[string]any) { m.items[m.key(item)] = item }
	for i, c := range w.Cards {
		put(map[string]any{"PK": s(w.ID), "SK": s("CARD#" + c.ID), "ID": s(c.ID), "Name": s(c.Name), "Description": s(c.Description), "Order": n(float64(i+1) * 1000)})
		for j, t := range c.Tasks {
			put(map[string]any{"PK": s(w.ID), "SK": s("TASK#" + c.ID + "#" + t.ID), "ID": s(t.ID), "CardID": s(c.ID), "Name": s(t.Name), "Description": s(t.Description), "Status": s(t.Status), "Order": n(float64(j+1) * 1000)})
		}
	}
}

func ptr(s string) *string { return &s }

var cases = []struct {
	name   string
	change Change
}{
	{"create card", Change{Type: CreateCard, BoardID: b1, CardID: newCard, Name: ptr("Ideas")}},
	{"create card with a taken id", Change{Type: CreateCard, BoardID: b1, CardID: c1, Name: ptr("Ideas")}},
	{"create card with a bad id", Change{Type: CreateCard, BoardID: b1, CardID: "x", Name: ptr("Ideas")}},
	{"create card without a board", Change{Type: CreateCard, CardID: newCard, Name: ptr("Ideas")}},
	{"update card", Change{Type: UpdateCard, BoardID: b1, CardID: c1, Name: ptr("Later"), Description: ptr("notes\nmore")}},
	{"update a card's name only", Change{Type: UpdateCard, BoardID: b1, CardID: c1, Name: ptr("Later")}},
	{"update a card's description only", Change{Type: UpdateCard, BoardID: b1, CardID: c1, Description: ptr("")}},
	{"update a card with nothing", Change{Type: UpdateCard, BoardID: b1, CardID: c1}},
	{"update a card's status", Change{Type: UpdateCard, BoardID: b1, CardID: c1, Status: ptr("done")}},
	{"update missing card", Change{Type: UpdateCard, BoardID: b1, CardID: missing, Name: ptr("x")}},
	{"delete card with tasks", Change{Type: DeleteCard, BoardID: b1, CardID: c1}},
	{"delete missing card", Change{Type: DeleteCard, BoardID: b1, CardID: missing}},
	{"delete card without an id", Change{Type: DeleteCard, BoardID: b1}},
	{"move card to the end", Change{Type: MoveCard, BoardID: b1, CardID: c1, BeforeID: ptr(c3)}},
	{"move card to the front", Change{Type: MoveCard, BoardID: b1, CardID: c3, AfterID: ptr(c1)}},
	{"move card between", Change{Type: MoveCard, BoardID: b1, CardID: c3, BeforeID: ptr(c1), AfterID: ptr(c2)}},
	{"move card after a missing card, before another", Change{Type: MoveCard, BoardID: b1, CardID: c1, BeforeID: ptr(missingID), AfterID: ptr(c3)}},
	{"move card before a missing card, after another", Change{Type: MoveCard, BoardID: b1, CardID: c3, BeforeID: ptr(c1), AfterID: ptr(missingID)}},
	{"move card beside itself", Change{Type: MoveCard, BoardID: b1, CardID: c1, BeforeID: ptr(c1)}},
	{"move card nowhere", Change{Type: MoveCard, BoardID: b1, CardID: c3}},
	{"move card with empty neighbours", Change{Type: MoveCard, BoardID: b1, CardID: c3, BeforeID: ptr(""), AfterID: ptr("")}},
	{"move missing card", Change{Type: MoveCard, BoardID: b1, CardID: missing, BeforeID: ptr(c1)}},
	{"move missing card nowhere", Change{Type: MoveCard, BoardID: b1, CardID: missing}},
	{"move card beside a missing card", Change{Type: MoveCard, BoardID: b1, CardID: c1, BeforeID: ptr(missingID)}},
	{"move missing card beside a missing card", Change{Type: MoveCard, BoardID: b1, CardID: missing, AfterID: ptr(missingID)}},
	{"create task", Change{Type: CreateTask, BoardID: b1, CardID: c1, TaskID: newTask, Name: ptr("Buy milk")}},
	{"create task in an empty card", Change{Type: CreateTask, BoardID: b1, CardID: c3, TaskID: newTask, Name: ptr("Buy milk")}},
	{"create task in a missing card", Change{Type: CreateTask, BoardID: b1, CardID: missing, TaskID: newTask, Name: ptr("Buy milk")}},
	{"create task with a taken id", Change{Type: CreateTask, BoardID: b1, CardID: c1, TaskID: t1, Name: ptr("Buy milk")}},
	{"create task with the id of a task in another card", Change{Type: CreateTask, BoardID: b1, CardID: c2, TaskID: t1, Name: ptr("Buy milk")}},
	{"create task with a bad id", Change{Type: CreateTask, BoardID: b1, CardID: c1, TaskID: "TASK#x", Name: ptr("Buy milk")}},
	{"update task", Change{Type: UpdateTask, BoardID: b1, CardID: c1, TaskID: t1, Name: ptr("Write more"), Description: ptr("d"), Status: ptr("done")}},
	{"update a task's status only", Change{Type: UpdateTask, BoardID: b1, CardID: c1, TaskID: t1, Status: ptr("done")}},
	{"update a task with nothing", Change{Type: UpdateTask, BoardID: b1, CardID: c1, TaskID: t1}},
	{"update a task in the card it was moved from", Change{Type: UpdateTask, BoardID: b1, CardID: c2, TaskID: t1, Name: ptr("Moved")}},
	{"update missing task", Change{Type: UpdateTask, BoardID: b1, CardID: c1, TaskID: missing, Name: ptr("x"), Status: ptr("done")}},
	{"update task without a task id", Change{Type: UpdateTask, BoardID: b1, CardID: c1, Name: ptr("x")}},
	{"move task to the end", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c1, BeforeID: ptr(t2)}},
	{"move task to the front", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t2, NewCardID: c1, AfterID: ptr(t1)}},
	{"move task within its card, nowhere", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t2, NewCardID: c1}},
	{"move task to another card's end", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c2, BeforeID: ptr(t3)}},
	{"move task to another card's front", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t2, NewCardID: c2, AfterID: ptr(t3)}},
	{"move task to another card, nowhere", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c2}},
	{"move task to an empty card", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c3}},
	{"move task between", Change{Type: MoveTask, BoardID: b1, CardID: c2, TaskID: t3, NewCardID: c1, BeforeID: ptr(t1), AfterID: ptr(t2)}},
	{"move task from the card it was moved from", Change{Type: MoveTask, BoardID: b1, CardID: c2, TaskID: t1, NewCardID: c3}},
	{"move task before a missing task, after another", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c2, BeforeID: ptr(missingID), AfterID: ptr(t3)}},
	{"delete a task from the card it was moved from", Change{Type: DeleteTask, BoardID: b1, CardID: c2, TaskID: t1}},
	{"move missing task", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: missing, NewCardID: c2}},
	{"move task to a missing card", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: missing}},
	{"move task beside a missing task", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: c2, BeforeID: ptr(missingID)}},
	{"move task to a missing card beside a task", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1, NewCardID: missing, BeforeID: ptr(t3)}},
	{"move task without a new card", Change{Type: MoveTask, BoardID: b1, CardID: c1, TaskID: t1}},
	{"delete task", Change{Type: DeleteTask, BoardID: b1, CardID: c1, TaskID: t1}},
	{"delete missing task", Change{Type: DeleteTask, BoardID: b1, CardID: c1, TaskID: missing}},
	{"clean up", Change{Type: CleanUp, BoardID: b1, TaskIDs: []string{t2}}},
	{"clean up deletes only the tasks it names", Change{Type: CleanUp, BoardID: b1, TaskIDs: []string{t1, missingID}}},
	{"clean up with nothing done", Change{Type: CleanUp, BoardID: b1, TaskIDs: []string{}}},
	{"clean up without task ids", Change{Type: CleanUp, BoardID: b1}},
	{"clean up without a board", Change{Type: CleanUp}},
	{"unknown type", Change{Type: "renameBoard", BoardID: b1}},
}

func outcome(err error) string {
	if err == nil {
		return "ok"
	}
	return fmt.Sprintf("%d %s", Status(err), err)
}

func TestReduceAnswersLikeTheStore(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := &memTable{}
			table.load(seedBoard())
			fake := awsfake.New(t, table.respond)
			storeErr := NewStore(fake.DynamoDB(), "Skipli").Apply(context.Background(), tc.change)

			file := seedBoard()
			fileErr := Reduce(&file, tc.change)

			if got, want := outcome(fileErr), outcome(storeErr); got != want {
				t.Fatalf("Reduce answered %q, the Store %q", got, want)
			}
			if want := table.snapshot(b1); !reflect.DeepEqual(file, want) {
				t.Errorf("after %s\nReduce left %s\nthe Store left %s", tc.name, dump(file), dump(want))
			}
		})
	}
}

// A refused change leaves the board file as it was.
func TestReduceLeavesTheBoardAloneWhenRefusing(t *testing.T) {
	for _, tc := range cases {
		file := seedBoard()
		if err := Reduce(&file, tc.change); err != nil && !reflect.DeepEqual(file, seedBoard()) {
			t.Errorf("%s: refused with %v but changed the board to %s", tc.name, err, dump(file))
		}
	}
}

// Applying all the valid cases in a row keeps the two in step too.
func TestReduceFollowsTheStoreThroughASequence(t *testing.T) {
	table := &memTable{}
	table.load(seedBoard())
	fake := awsfake.New(t, table.respond)
	store := NewStore(fake.DynamoDB(), "Skipli")
	file := seedBoard()
	for _, tc := range cases {
		storeErr := store.Apply(context.Background(), tc.change)
		fileErr := Reduce(&file, tc.change)
		if outcome(fileErr) != outcome(storeErr) {
			t.Fatalf("%s: Reduce answered %q, the Store %q", tc.name, outcome(fileErr), outcome(storeErr))
		}
		if want := table.snapshot(b1); !reflect.DeepEqual(file, want) {
			t.Fatalf("after %s\nReduce left %s\nthe Store left %s", tc.name, dump(file), dump(want))
		}
	}
}

func dump(w board.Whole) string {
	var b strings.Builder
	for _, c := range w.Cards {
		fmt.Fprintf(&b, "\n  card %s %q %q", c.ID[len(c.ID)-2:], c.Name, c.Description)
		for _, t := range c.Tasks {
			fmt.Fprintf(&b, "\n    task %s %s %q %q %s", t.ID[len(t.ID)-2:], t.CardID[len(t.CardID)-2:], t.Name, t.Description, t.Status)
		}
	}
	return b.String()
}
