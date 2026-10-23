package boardchange

import (
	"net/http"
	"slices"

	"github.com/Schiffer116/skipli/api/internal/board"
)

// Reduce applies a change to a board held in memory, such as a board file,
// the way Store.Apply applies it to the table: the same refusals, and the
// same cards and tasks left. contract_test.go holds the two to that. A
// refused change leaves b as it was.
//
// Only moves that name neighbours that sit next to each other are the same
// on both: the table places an item by a number between its neighbours'
// numbers, so far-apart neighbours give it a place the cards' order doesn't
// show, which a list can't copy. The CLI and the app only send adjacent ones.
func Reduce(b *board.Whole, c Change) error {
	if err := c.validate(); err != nil {
		return err
	}
	cards := slices.Clone(b.Cards)
	for i := range cards {
		cards[i].Tasks = slices.Clone(cards[i].Tasks)
	}
	if err := reduce(&cards, c); err != nil {
		return err
	}
	b.Cards = cards
	return nil
}

func cardIndex(cards []board.Card, id string) int {
	return slices.IndexFunc(cards, func(c board.Card) bool { return c.ID == id })
}

func taskIndex(tasks []board.Task, id string) int {
	return slices.IndexFunc(tasks, func(t board.Task) bool { return t.ID == id })
}

func reduce(cards *[]board.Card, c Change) error {
	ci := cardIndex(*cards, c.CardID)
	switch c.Type {
	case CreateCard:
		if ci >= 0 {
			return refuse(http.StatusConflict, "an item with this id already exists")
		}
		*cards = append(*cards, board.Card{ID: c.CardID, Name: c.Name, Tasks: []board.Task{}})
	case UpdateCard:
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		(*cards)[ci].Name, (*cards)[ci].Description = c.Name, c.Description
	case DeleteCard:
		if ci >= 0 {
			*cards = slices.Delete(*cards, ci, ci+1)
		}
	case MoveCard:
		before, after := neighbour(c.BeforeID), neighbour(c.AfterID)
		if before == "" && after == "" {
			return nil
		}
		ids := make([]string, 0, len(*cards))
		for _, card := range *cards {
			if card.ID != c.CardID {
				ids = append(ids, card.ID)
			}
		}
		at, err := place(ids, before, after)
		if err != nil {
			return err
		}
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		card := (*cards)[ci]
		rest := slices.Delete(slices.Clone(*cards), ci, ci+1)
		*cards = slices.Insert(rest, at, card)
	case CreateTask:
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		if taskIndex((*cards)[ci].Tasks, c.TaskID) >= 0 {
			return refuse(http.StatusConflict, "a task with this id already exists")
		}
		(*cards)[ci].Tasks = append((*cards)[ci].Tasks, board.Task{ID: c.TaskID, CardID: c.CardID, Name: c.Name, Status: "pending"})
	case UpdateTask:
		ti := -1
		if ci >= 0 {
			ti = taskIndex((*cards)[ci].Tasks, c.TaskID)
		}
		if ti < 0 {
			return refuse(http.StatusNotFound, "task not found")
		}
		t := &(*cards)[ci].Tasks[ti]
		t.Name, t.Description, t.Status = c.Name, c.Description, c.Status
	case DeleteTask:
		if ci >= 0 {
			if ti := taskIndex((*cards)[ci].Tasks, c.TaskID); ti >= 0 {
				(*cards)[ci].Tasks = slices.Delete((*cards)[ci].Tasks, ti, ti+1)
			}
		}
	case MoveTask:
		return moveTask(*cards, c)
	case CleanUp:
		for i := range *cards {
			(*cards)[i].Tasks = slices.DeleteFunc((*cards)[i].Tasks, func(t board.Task) bool { return t.Status == "done" })
		}
	default:
		return refuse(http.StatusBadRequest, "unknown change type")
	}
	return nil
}

func moveTask(cards []board.Card, c Change) error {
	ci := cardIndex(cards, c.CardID)
	ti := -1
	if ci >= 0 {
		ti = taskIndex(cards[ci].Tasks, c.TaskID)
	}
	if ti < 0 {
		return refuse(http.StatusNotFound, "task not found")
	}
	task := cards[ci].Tasks[ti]

	ni := cardIndex(cards, c.NewCardID)
	// The table looks the neighbours up in the new card, whether it exists or
	// not, and the task itself is one of them only if it stays in its card.
	var ids []string
	if ni >= 0 {
		for _, t := range cards[ni].Tasks {
			if t.ID != c.TaskID || c.NewCardID != c.CardID {
				ids = append(ids, t.ID)
			}
		}
	}
	before, after := neighbour(c.BeforeID), neighbour(c.AfterID)
	at, err := place(ids, before, after)
	if err != nil {
		return err
	}
	if ni < 0 {
		return refuse(http.StatusNotFound, "card not found")
	}

	cards[ci].Tasks = slices.Delete(cards[ci].Tasks, ti, ti+1)
	task.CardID = c.NewCardID
	rest := slices.DeleteFunc(slices.Clone(cards[ni].Tasks), func(t board.Task) bool { return t.ID == c.TaskID })
	cards[ni].Tasks = slices.Insert(rest, at, task)
	return nil
}

func neighbour(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

// place is where among ids an item goes that is to follow before and to
// precede after: right after before, else right before after, else (the table
// gives it the number 0, below any other) the front. A neighbour that isn't in
// ids is refused.
func place(ids []string, before, after string) (int, error) {
	at := func(id string) (int, error) {
		i := slices.Index(ids, id)
		if i < 0 {
			return 0, refuse(http.StatusNotFound, "neighbour not found")
		}
		return i, nil
	}
	var bi, ai int
	var err error
	if before != "" {
		if bi, err = at(before); err != nil {
			return 0, err
		}
	}
	if after != "" {
		if ai, err = at(after); err != nil {
			return 0, err
		}
	}
	switch {
	case before != "":
		return bi + 1, nil
	case after != "":
		return ai, nil
	}
	return 0, nil
}
