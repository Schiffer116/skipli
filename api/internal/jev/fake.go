package jev

import (
	"context"
	"strings"
	"unicode"
)

// Fake stands in for Jev on a developer's machine and in tests, where there
// is no key: a task scores the share of the criterion's words found in its
// name or description, so "urgent" puts the tasks that say urgent first.
type Fake struct{}

func (Fake) Judge(_ context.Context, criterion string, tasks []Task) (Verdict, error) {
	want := words(criterion)
	yes := make([]float64, len(tasks))
	for i, task := range tasks {
		have := map[string]bool{}
		for _, word := range words(task.Name + " " + task.Description) {
			have[word] = true
		}
		found := 0
		for _, word := range want {
			if have[word] {
				found++
			}
		}
		if len(want) > 0 {
			yes[i] = float64(found) / float64(len(want))
		}
	}
	return Verdict{Model: "fake", Yes: yes}, nil
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
