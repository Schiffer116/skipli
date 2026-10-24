package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultBaseURL is TypeSafe's own API. OpenRouter
	// (https://openrouter.ai/api) and Vercel's AI Gateway take the same
	// requests under their own base URL, key and model name.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel is pinned, so a new Jev release is a deliberate change.
	DefaultModel = "jev-1.14.0"

	// deadline bounds a whole sort, retry included, well inside the
	// Lambda's timeout.
	deadline = 10 * time.Second
	// Each task's text is cut to these many characters, and the state to
	// maxState bytes (about 15k tokens), so a request stays inside Jev's
	// 32k tokens per question without its slower long-context mode.
	maxName        = 200
	maxDescription = 500
	maxState       = 60_000
)

// Client asks Jev over HTTP.
type Client struct {
	BaseURL string
	Model   string
	// Key is read on each request, so one set after the API started is
	// picked up.
	Key  func(context.Context) (string, error)
	HTTP *http.Client
}

type request struct {
	Model     string              `json:"model"`
	State     state               `json:"state"`
	Questions map[string]question `json:"questions"`
}

type state struct {
	Criterion string               `json:"criterion"`
	Tasks     map[string]stateTask `json:"tasks"`
}

type stateTask struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

type question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type response struct {
	Model   string `json:"model"`
	Answers map[string]struct {
		Type string   `json:"type"`
		Noul *float64 `json:"noul"`
	} `json:"answers"`
}

// label names the i-th task in the state, "T000" onwards: task IDs are long
// UUIDs that would only cost tokens.
func label(i int) string { return fmt.Sprintf("T%03d", i) }

func (c *Client) Judge(ctx context.Context, criterion string, tasks []Task) (Verdict, error) {
	key, err := c.Key(ctx)
	if err != nil {
		return Verdict{}, err
	}
	body, err := json.Marshal(c.request(criterion, tasks))
	if err != nil {
		return Verdict{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	res, err := c.post(ctx, key, body)
	if err != nil {
		return Verdict{}, err
	}

	verdict := Verdict{Model: res.Model, Yes: make([]float64, len(tasks))}
	for i := range tasks {
		answer, ok := res.Answers[label(i)]
		if !ok || answer.Noul == nil {
			return Verdict{}, fmt.Errorf("jev gave no answer for %s", label(i))
		}
		verdict.Yes[i] = *answer.Noul
	}
	return verdict, nil
}

func (c *Client) request(criterion string, tasks []Task) request {
	r := request{
		Model:     c.Model,
		State:     state{Criterion: criterion, Tasks: map[string]stateTask{}},
		Questions: map[string]question{},
	}
	for i, task := range tasks {
		r.State.Tasks[label(i)] = stateTask{
			Name:        truncate(task.Name, maxName),
			Description: truncate(task.Description, maxDescription),
			Status:      task.Status,
		}
		r.Questions[label(i)] = question{
			Type:         "noul",
			Instructions: fmt.Sprintf("Ordering these tasks by the criterion, does task %s belong near the top?", label(i)),
			Criteria: map[string]string{
				"true":  "The criterion puts this task ahead of most of the others.",
				"false": "The criterion puts this task behind most of the others.",
			},
		}
	}
	return r
}

// TooLarge reports whether tasks are too many to judge in one request, to
// turn a card away before asking.
func TooLarge(criterion string, tasks []Task) bool {
	state, _ := json.Marshal((&Client{}).request(criterion, tasks).State)
	return len(state) > maxState
}

// post sends the request, and once more if Jev is busy and says when to
// come back soon enough to still answer within ctx.
func (c *Client) post(ctx context.Context, key string, body []byte) (response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.BaseURL, "/")+"/v1/systemone", bytes.NewReader(body))
		if err != nil {
			return response{}, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			if attempt == 0 {
				continue
			}
			return response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if err != nil {
			return response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		requestID := res.Header.Get("X-Typesafe-Request-Id")

		switch {
		case res.StatusCode == http.StatusOK:
			var out response
			if err := json.Unmarshal(raw, &out); err != nil {
				return response{}, fmt.Errorf("jev request %s: %w", requestID, err)
			}
			return out, nil
		case res.StatusCode == http.StatusRequestEntityTooLarge:
			return response{}, ErrTooLarge
		case retryable(res.StatusCode):
			wait := retryAfter(res.Header.Get("Retry-After"))
			if until, ok := ctx.Deadline(); attempt == 0 && (!ok || time.Until(until) > wait) {
				log.Printf("jev request %s: %d, trying again in %s", requestID, res.StatusCode, wait)
				select {
				case <-time.After(wait):
					continue
				case <-ctx.Done():
				}
			}
			return response{}, fmt.Errorf("%w: %d (request %s)", ErrUnavailable, res.StatusCode, requestID)
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			// Logged loudly: the key is wrong or revoked, not Jev busy.
			log.Printf("jev rejected the key: %d (request %s)", res.StatusCode, requestID)
			return response{}, fmt.Errorf("%w: key rejected", ErrUnavailable)
		default:
			return response{}, fmt.Errorf("jev request %s: %d %s", requestID, res.StatusCode, bytes.TrimSpace(raw))
		}
	}
}

// retryable is what TypeSafe's own client retries.
func retryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	}
	return status >= 500
}

// retryAfter reads a Retry-After header in seconds or as a date, a second
// when it's missing.
func retryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil {
		return max(time.Duration(seconds)*time.Second, 0)
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	return time.Second
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
