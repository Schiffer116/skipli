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
// Known difference: the table places an item by a number between its
// neighbours' numbers, so neighbours that don't sit next to each other give
// it a place the cards' order doesn't show, which a list can't copy. The
// moves of the CLI and the app name adjacent neighbours, which both agree on.
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
		*cards = append(*cards, board.Card{ID: c.CardID, Name: value(c.Name), Tasks: []board.Task{}})
	case UpdateCard:
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		card := &(*cards)[ci]
		setField(&card.Name, c.Name)
		setField(&card.Description, c.Description)
	case DeleteCard:
		if ci >= 0 {
			*cards = slices.Delete(*cards, ci, ci+1)
		}
	case MoveCard:
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		card := (*cards)[ci]
		rest := slices.Delete(slices.Clone(*cards), ci, ci+1)
		ids := make([]string, len(rest))
		for i, other := range rest {
			ids[i] = other.ID
		}
		*cards = slices.Insert(rest, slot(ids, c.BeforeID, c.AfterID), card)
	case CreateTask:
		if _, _, ok := findTask(*cards, c.TaskID); ok {
			return refuse(http.StatusConflict, "a task with this id already exists")
		}
		if ci < 0 {
			return refuse(http.StatusNotFound, "card not found")
		}
		(*cards)[ci].Tasks = append((*cards)[ci].Tasks, board.Task{ID: c.TaskID, CardID: c.CardID, Name: value(c.Name), Status: "pending"})
	case UpdateTask:
		fc, ti, ok := findTask(*cards, c.TaskID)
		if !ok {
			return refuse(http.StatusNotFound, "task not found")
		}
		t := &(*cards)[fc].Tasks[ti]
		setField(&t.Name, c.Name)
		setField(&t.Description, c.Description)
		setField(&t.Status, c.Status)
	case DeleteTask:
		if fc, ti, ok := findTask(*cards, c.TaskID); ok {
			(*cards)[fc].Tasks = slices.Delete((*cards)[fc].Tasks, ti, ti+1)
		}
	case MoveTask:
		return moveTask(*cards, c)
	case CleanUp:
		for i := range *cards {
			(*cards)[i].Tasks = slices.DeleteFunc((*cards)[i].Tasks, func(t board.Task) bool { return slices.Contains(c.TaskIDs, t.ID) })
		}
	default:
		return refuse(http.StatusBadRequest, "unknown change type")
	}
	return nil
}

// setField sets *field to the value of an update's field, if it has one.
func setField(field *string, v *string) {
	if v != nil {
		*field = *v
	}
}

// findTask finds a task by its id, in whichever card it is in now, as the
// table does.
func findTask(cards []board.Card, id string) (card, task int, ok bool) {
	for ci, c := range cards {
		if ti := taskIndex(c.Tasks, id); ti >= 0 {
			return ci, ti, true
		}
	}
	return 0, 0, false
}

func moveTask(cards []board.Card, c Change) error {
	ci, ti, ok := findTask(cards, c.TaskID)
	if !ok {
		return refuse(http.StatusNotFound, "task not found")
	}
	ni := cardIndex(cards, c.NewCardID)
	if ni < 0 {
		return refuse(http.StatusNotFound, "card not found")
	}
	task := cards[ci].Tasks[ti]
	cards[ci].Tasks = slices.Delete(cards[ci].Tasks, ti, ti+1)

	rest := cards[ni].Tasks
	ids := make([]string, len(rest))
	for i, t := range rest {
		ids[i] = t.ID
	}
	task.CardID = c.NewCardID
	cards[ni].Tasks = slices.Insert(slices.Clone(rest), slot(ids, c.BeforeID, c.AfterID), task)
	return nil
}

// slot is where among ids an item goes that is to follow beforeID or, if that
// isn't there, to precede afterID, else at the end. A neighbour that is gone
// is never a reason to refuse, as in cards.go's place.
func slot(ids []string, beforeID, afterID *string) int {
	if beforeID != nil {
		if i := slices.Index(ids, *beforeID); i >= 0 {
			return i + 1
		}
	}
	if afterID != nil {
		if i := slices.Index(ids, *afterID); i >= 0 {
			return i
		}
	}
	return len(ids)
}
