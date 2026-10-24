package cardsort

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/jev"
)

func s(v string) map[string]any { return map[string]any{"S": v} }
func n(v string) map[string]any { return map[string]any{"N": v} }

func task(id, name, order string) map[string]any {
	return map[string]any{
		"PK": s("b1"), "SK": s("TASK#c1#" + id), "ID": s(id), "CardID": s("c1"),
		"Name": s(name), "Description": s(""), "Status": s("pending"), "Order": n(order),
	}
}

// cardC1 serves card c1 with tasks, in key order rather than their Order;
// limited makes the sort count refuse, as when someone sorts too often.
func cardC1(t *testing.T, limited map[string]any, tasks ...any) *awsfake.Server {
	return awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "GetItem":
			if body["Key"].(map[string]any)["SK"].(map[string]any)["S"] == "CARD#c1" {
				return 200, map[string]any{"Item": map[string]any{"PK": s("b1"), "SK": s("CARD#c1")}}
			}
		case "Query":
			return 200, map[string]any{"Items": tasks}
		case "UpdateItem":
			if limited != nil {
				return 400, map[string]any{
					"__type": "com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException",
					"Item":   limited,
				}
			}
		}
		return 200, map[string]any{}
	})
}

func sort(t *testing.T, fake *awsfake.Server, judge jev.Judge, card, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(fake.DynamoDB(), "Skipli", nil, judge)
	req := httptest.NewRequest("POST", "/boards/b1/cards/"+card+"/sort", strings.NewReader(body))
	req.SetPathValue("boardId", "b1")
	req.SetPathValue("cardId", card)
	req = req.WithContext(auth.ContextWithUser(req.Context(), auth.User{ID: "u1"}))
	rec := httptest.NewRecorder()
	h.sort(rec, req)
	return rec
}

var three = []any{
	task("a", "water plants", "1000"),
	task("b", "urgent outage", "3000"),
	task("c", "urgent email", "2000"),
}

func TestSortGivesTheJudgedOrderAndCountsTheSort(t *testing.T) {
	fake := cardC1(t, nil, three...)
	rec := sort(t, fake, jev.Fake{}, "c1", `{"criterion":"  urgent outage "}`)
	if want := `{"order":["b","c","a"],"model":"fake"}`; rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("got %d %s, want %s", rec.Code, rec.Body, want)
	}
	updates := fake.Calls("UpdateItem")
	if len(updates) != 1 || !strings.HasPrefix(updates[0].Body["Key"].(map[string]any)["PK"].(map[string]any)["S"].(string), "USER#u1") {
		t.Errorf("counted %v", updates)
	}
	if calls := fake.Calls("PutItem", "DeleteItem", "TransactWriteItems", "BatchWriteItem"); len(calls) != 0 {
		t.Errorf("sort wrote to the board: %v", calls)
	}
}

func TestSortRefusals(t *testing.T) {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	recent := map[string]any{"Sorts": n("1"), "LastAt": n(now)}
	usedUp := map[string]any{"Sorts": n("30"), "LastAt": n("0")}
	for _, tc := range []struct {
		name    string
		card    string
		body    string
		limited map[string]any
		judge   jev.Judge
		want    int
	}{
		{"no criterion", "c1", `{"criterion":"   "}`, nil, jev.Fake{}, 400},
		{"long criterion", "c1", `{"criterion":"` + strings.Repeat("x", 201) + `"}`, nil, jev.Fake{}, 400},
		{"missing card", "gone", `{"criterion":"x"}`, nil, jev.Fake{}, 404},
		{"too soon", "c1", `{"criterion":"x"}`, recent, jev.Fake{}, 429},
		{"hour used up", "c1", `{"criterion":"x"}`, usedUp, jev.Fake{}, 429},
		{"no key", "c1", `{"criterion":"x"}`, nil, failing{jev.ErrNotConfigured}, 503},
		{"jev down", "c1", `{"criterion":"x"}`, nil, failing{jev.ErrUnavailable}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := sort(t, cardC1(t, tc.limited, three...), tc.judge, tc.card, tc.body)
			if rec.Code != tc.want {
				t.Errorf("status %d %s, want %d", rec.Code, strings.TrimSpace(rec.Body.String()), tc.want)
			}
			if tc.want == 429 && rec.Header().Get("Retry-After") == "" {
				t.Error("no Retry-After")
			}
		})
	}
}

func TestACardOfOneTaskIsNotJudged(t *testing.T) {
	fake := cardC1(t, nil, task("a", "only", "1000"))
	rec := sort(t, fake, failing{jev.ErrUnavailable}, "c1", `{"criterion":"x"}`)
	if want := `{"order":["a"],"model":""}`; rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if len(fake.Calls("UpdateItem")) != 0 {
		t.Error("counted a sort that didn't ask Jev")
	}
}

type failing struct{ err error }

func (f failing) Judge(context.Context, string, []jev.Task) (jev.Verdict, error) {
	return jev.Verdict{}, f.err
}
