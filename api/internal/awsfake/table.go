package awsfake

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Table is an in-memory DynamoDB table, for tests that need writes to be
// read back. It understands the operations and expressions this API uses:
// keys PK and SK, SET updates, attribute_exists / attribute_not_exists
// conditions, and = / begins_with comparisons joined by AND. Projections
// are ignored and queries return everything in one page.
type Table struct {
	mu    sync.Mutex
	items map[string]map[string]any
}

func NewTable() *Table {
	return &Table{items: map[string]map[string]any{}}
}

// Put stores an item as it is, for seeding a test.
func (t *Table) Put(item map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items[key(item)] = item
}

// Items returns every item, in key order.
func (t *Table) Items() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sorted(func(map[string]any) bool { return true })
}

func (t *Table) sorted(keep func(map[string]any) bool) []map[string]any {
	keys := make([]string, 0, len(t.items))
	for k, item := range t.items {
		if keep(item) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	items := make([]map[string]any, len(keys))
	for i, k := range keys {
		items[i] = t.items[k]
	}
	return items
}

func str(v any) string {
	s, _ := v.(map[string]any)["S"].(string)
	return s
}

func key(item map[string]any) string {
	return str(item["PK"]) + "\x00" + str(item["SK"])
}

// Respond serves the table's operations, for New.
func (t *Table) Respond(op string, body map[string]any) (int, any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	names, _ := body["ExpressionAttributeNames"].(map[string]any)
	values, _ := body["ExpressionAttributeValues"].(map[string]any)
	conditionFailed := map[string]any{"__type": "ConditionalCheckFailedException", "message": "The conditional request failed"}

	switch op {
	case "GetItem":
		if item, ok := t.items[key(body["Key"].(map[string]any))]; ok {
			return 200, map[string]any{"Item": item}
		}
		return 200, map[string]any{}

	case "PutItem", "UpdateItem", "DeleteItem":
		w := write{op: strings.TrimSuffix(op, "Item"), body: body}
		if !t.holds(w.target(), body["ConditionExpression"], names, values) {
			return 400, conditionFailed
		}
		if err := t.apply(w, names, values); err != nil {
			return 400, map[string]any{"__type": "ValidationException", "message": err.Error()}
		}
		return 200, map[string]any{}

	case "Query":
		keyCond := body["KeyConditionExpression"]
		filter := body["FilterExpression"]
		items := t.sorted(func(item map[string]any) bool {
			return eval(item, keyCond, names, values) && eval(item, filter, names, values)
		})
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return 200, map[string]any{"Items": out, "Count": len(out)}

	case "TransactWriteItems":
		var writes []write
		for _, raw := range body["TransactItems"].([]any) {
			for kind, b := range raw.(map[string]any) {
				writes = append(writes, write{op: kind, body: b.(map[string]any)})
			}
		}
		reasons := make([]any, len(writes))
		failed := false
		for i, w := range writes {
			n, _ := w.body["ExpressionAttributeNames"].(map[string]any)
			v, _ := w.body["ExpressionAttributeValues"].(map[string]any)
			code := "None"
			if !t.holds(w.target(), w.body["ConditionExpression"], n, v) {
				code, failed = "ConditionalCheckFailed", true
			}
			reasons[i] = map[string]any{"Code": code}
		}
		if failed {
			return 400, map[string]any{
				"__type":              "TransactionCanceledException",
				"Message":             "Transaction cancelled",
				"CancellationReasons": reasons,
			}
		}
		for _, w := range writes {
			n, _ := w.body["ExpressionAttributeNames"].(map[string]any)
			v, _ := w.body["ExpressionAttributeValues"].(map[string]any)
			if err := t.apply(w, n, v); err != nil {
				return 400, map[string]any{"__type": "ValidationException", "message": err.Error()}
			}
		}
		return 200, map[string]any{}

	case "BatchWriteItem":
		for _, requests := range body["RequestItems"].(map[string]any) {
			for _, raw := range requests.([]any) {
				request := raw.(map[string]any)
				if put, ok := request["PutRequest"].(map[string]any); ok {
					item := put["Item"].(map[string]any)
					t.items[key(item)] = item
				}
				if del, ok := request["DeleteRequest"].(map[string]any); ok {
					delete(t.items, key(del["Key"].(map[string]any)))
				}
			}
		}
		return 200, map[string]any{"UnprocessedItems": map[string]any{}}
	}
	return 400, map[string]any{"__type": "UnknownOperationException", "message": op}
}

// write is one Put, Update, Delete or ConditionCheck, alone or in a
// transaction.
type write struct {
	op   string
	body map[string]any
}

func (w write) target() string {
	if w.op == "Put" {
		return key(w.body["Item"].(map[string]any))
	}
	return key(w.body["Key"].(map[string]any))
}

func (t *Table) holds(k string, cond any, names, values map[string]any) bool {
	item, ok := t.items[k]
	if !ok {
		item = nil
	}
	return eval(item, cond, names, values)
}

var setClause = regexp.MustCompile(`^\s*(\S+)\s*=\s*(\S+)\s*$`)

func (t *Table) apply(w write, names, values map[string]any) error {
	switch w.op {
	case "Put":
		item := w.body["Item"].(map[string]any)
		t.items[key(item)] = item
	case "Delete":
		delete(t.items, w.target())
	case "Update":
		k := w.target()
		item := map[string]any{}
		for name, v := range t.items[k] {
			item[name] = v
		}
		for name, v := range w.body["Key"].(map[string]any) {
			item[name] = v
		}
		update, _ := w.body["UpdateExpression"].(string)
		update = strings.TrimSpace(update)
		if !strings.HasPrefix(update, "SET ") {
			return fmt.Errorf("unsupported update %q", update)
		}
		for _, clause := range strings.Split(strings.TrimPrefix(update, "SET "), ",") {
			m := setClause.FindStringSubmatch(clause)
			if m == nil {
				return fmt.Errorf("unsupported update %q", update)
			}
			item[resolve(m[1], names)] = values[m[2]]
		}
		t.items[k] = item
	}
	return nil
}

func resolve(name string, names map[string]any) string {
	if strings.HasPrefix(name, "#") {
		return names[name].(string)
	}
	return name
}

var (
	exists     = regexp.MustCompile(`^attribute_(not_)?exists\s*\(\s*(\S+?)\s*\)$`)
	equals     = regexp.MustCompile(`^(\S+)\s*=\s*(:\S+)$`)
	beginsWith = regexp.MustCompile(`^begins_with\s*\(\s*(\S+?)\s*,\s*(:\S+?)\s*\)$`)
)

// eval says whether item (nil when there is none) meets a condition, key
// condition or filter. No expression always holds.
func eval(item map[string]any, expr any, names, values map[string]any) bool {
	s, _ := expr.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if parts := splitAnd(s); len(parts) > 1 {
		for _, part := range parts {
			if !eval(item, part, names, values) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && closes(s) == len(s)-1 {
		return eval(item, s[1:len(s)-1], names, values)
	}
	if m := exists.FindStringSubmatch(s); m != nil {
		_, ok := item[resolve(m[2], names)]
		return ok == (m[1] == "")
	}
	if m := equals.FindStringSubmatch(s); m != nil {
		v, ok := item[resolve(m[1], names)]
		return ok && reflect.DeepEqual(v, values[m[2]])
	}
	if m := beginsWith.FindStringSubmatch(s); m != nil {
		return strings.HasPrefix(str(item[resolve(m[1], names)]), str(values[m[2]]))
	}
	panic(fmt.Sprintf("awsfake: unsupported expression %q", s))
}

// closes is the index of the parenthesis closing the one s starts with.
func closes(s string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitAnd splits s at the ANDs outside parentheses.
func splitAnd(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && strings.HasPrefix(s[i:], " AND ") {
			parts = append(parts, s[start:i])
			start = i + len(" AND ")
		}
	}
	return append(parts, s[start:])
}
