package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

var tasks = []Task{
	{ID: "a", Name: "Water plants", Status: "pending"},
	{ID: "b", Name: "Fix the urgent outage", Description: "prod is down", Status: "pending"},
	{ID: "c", Name: "Reply to email", Status: "done"},
}

// jev serves canned answers on /v1/systemone, recording each request.
func jev(t *testing.T, answer func(n int, w http.ResponseWriter, body request)) (*Client, *atomic.Int32) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request to %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q", got)
		}
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body: %v", err)
		}
		answer(n, w, body)
	}))
	t.Cleanup(srv.Close)
	return &Client{
		BaseURL: srv.URL + "/",
		Model:   DefaultModel,
		Key:     func(context.Context) (string, error) { return "k", nil },
		HTTP:    srv.Client(),
	}, &calls
}

// answers gives each task the probability in yes, by label.
func answers(w http.ResponseWriter, yes ...float64) {
	out := map[string]any{}
	for i, p := range yes {
		out[label(i)] = map[string]any{"type": "noul", "noul": p}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.14.0", "answers": out, "usage": map[string]int{"input_tokens": 10}})
}

func TestRankAsksOneNoulPerTaskInOneRequest(t *testing.T) {
	client, calls := jev(t, func(_ int, w http.ResponseWriter, body request) {
		if body.Model != DefaultModel || body.State.Criterion != "most urgent first" {
			t.Errorf("model %q, criterion %q", body.Model, body.State.Criterion)
		}
		if got := body.State.Tasks["T001"]; got.Name != "Fix the urgent outage" || got.Description != "prod is down" {
			t.Errorf("T001 = %+v", got)
		}
		for _, l := range []string{"T000", "T001", "T002"} {
			if q := body.Questions[l]; q.Type != "noul" || !strings.Contains(q.Instructions, l) || q.Criteria["true"] == "" {
				t.Errorf("question %s = %+v", l, q)
			}
		}
		answers(w, 0.2, 0.9, 0.2)
	})

	order, model, err := Rank(context.Background(), client, "most urgent first", tasks)
	if err != nil {
		t.Fatal(err)
	}
	// The tie between a and c keeps their order.
	if !slices.Equal(order, []string{"b", "a", "c"}) || model != "jev-1.14.0" {
		t.Errorf("order %v, model %q", order, model)
	}
	if calls.Load() != 1 {
		t.Errorf("%d requests, want 1", calls.Load())
	}
}

func TestBusyJevIsAskedOnceMore(t *testing.T) {
	client, calls := jev(t, func(n int, w http.ResponseWriter, _ request) {
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		answers(w, 0.1, 0.2, 0.3)
	})
	order, _, err := Rank(context.Background(), client, "x", tasks)
	if err != nil || !slices.Equal(order, []string{"c", "b", "a"}) || calls.Load() != 2 {
		t.Errorf("order %v, err %v, %d requests", order, err, calls.Load())
	}
}

func TestFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(w http.ResponseWriter)
		want   error
	}{
		{"still busy", func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
		}, ErrUnavailable},
		// Retrying would outlast the deadline.
		{"busy for long", func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}, ErrUnavailable},
		{"key rejected", func(w http.ResponseWriter) { w.WriteHeader(http.StatusUnauthorized) }, ErrUnavailable},
		{"too large", func(w http.ResponseWriter) { w.WriteHeader(http.StatusRequestEntityTooLarge) }, ErrTooLarge},
		{"malformed", func(w http.ResponseWriter) { http.Error(w, `{"detail":[]}`, http.StatusUnprocessableEntity) }, nil},
		{"missing answer", func(w http.ResponseWriter) { answers(w, 0.5, 0.5) }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, calls := jev(t, func(_ int, w http.ResponseWriter, _ request) { tc.answer(w) })
			_, _, err := Rank(context.Background(), client, "x", tasks)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if tc.name != "still busy" && calls.Load() != 1 {
				t.Errorf("%d requests, want 1", calls.Load())
			}
		})
	}
}

func TestAKeyThatIsNotSetIsNotSent(t *testing.T) {
	client, calls := jev(t, func(int, http.ResponseWriter, request) {})
	client.Key = func(context.Context) (string, error) { return "", ErrNotConfigured }
	if _, _, err := Rank(context.Background(), client, "x", tasks); !errors.Is(err, ErrNotConfigured) || calls.Load() != 0 {
		t.Errorf("err = %v after %d requests", err, calls.Load())
	}
}

func TestLongTextIsCut(t *testing.T) {
	long := Task{ID: "l", Name: strings.Repeat("é", 300), Description: strings.Repeat("d", 900)}
	sent := (&Client{}).request("x", []Task{long}).State.Tasks["T000"]
	if len([]rune(sent.Name)) != maxName || len(sent.Description) != maxDescription {
		t.Errorf("sent %d name runes, %d description bytes", len([]rune(sent.Name)), len(sent.Description))
	}

	var many []Task
	for i := range 200 {
		many = append(many, Task{ID: fmt.Sprint(i), Name: "task", Description: "short"})
	}
	if TooLarge("x", many) {
		t.Error("200 short tasks are too large")
	}
	for i := range many {
		many[i].Description = strings.Repeat("d", maxDescription)
	}
	if !TooLarge("x", many) {
		t.Error("200 long tasks fit")
	}
}

func TestFakePutsTasksThatSayTheCriterionFirst(t *testing.T) {
	order, model, err := Rank(context.Background(), Fake{}, "Urgent outage!", tasks)
	if err != nil || model != "fake" || !slices.Equal(order, []string{"b", "a", "c"}) {
		t.Errorf("order %v, model %q, err %v", order, model, err)
	}
}
