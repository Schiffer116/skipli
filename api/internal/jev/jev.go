// Package jev ranks a card's tasks by a criterion someone types, such as
// "most urgent first", using Jev, TypeSafe's hosted judgment model: one
// yes/no question (a Noul) per task, all in one request, answered with a
// calibrated probability that the task belongs near the top.
package jev

import (
	"cmp"
	"context"
	"errors"
	"slices"
)

// Task is what a judge sees of a task.
type Task struct {
	ID          string
	Name        string
	Description string
	Status      string
}

// Verdict is a judge's answer: for each task, in the order given, the
// probability that it belongs near the top of a list ordered by the
// criterion.
type Verdict struct {
	Model string
	Yes   []float64
}

type Judge interface {
	Judge(ctx context.Context, criterion string, tasks []Task) (Verdict, error)
}

var (
	// ErrNotConfigured is a deployed API whose key hasn't been set.
	ErrNotConfigured = errors.New("sorting is not configured")
	// ErrUnavailable is Jev refusing or failing to answer in time.
	ErrUnavailable = errors.New("jev is unavailable")
	// ErrTooLarge is a card too big to judge in one request.
	ErrTooLarge = errors.New("too many tasks to sort")
)

// Rank orders tasks' IDs by the judge's verdict, most likely first. Ties
// keep the order the tasks were given in.
func Rank(ctx context.Context, judge Judge, criterion string, tasks []Task) ([]string, string, error) {
	verdict, err := judge.Judge(ctx, criterion, tasks)
	if err != nil {
		return nil, "", err
	}
	if len(verdict.Yes) != len(tasks) {
		return nil, "", errors.New("judge answered for a different number of tasks")
	}
	at := make([]int, len(tasks))
	for i := range at {
		at[i] = i
	}
	slices.SortStableFunc(at, func(a, b int) int { return cmp.Compare(verdict.Yes[b], verdict.Yes[a]) })
	order := make([]string, len(tasks))
	for i, task := range at {
		order[i] = tasks[task].ID
	}
	return order, verdict.Model, nil
}
