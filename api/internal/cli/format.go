package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/Schiffer116/skipli/api/internal/board"
)

func writeBoards(w io.Writer, list board.ListResponse) {
	for _, b := range list.Boards {
		fmt.Fprintf(w, "board %s %s %d/%d done\n", b.ID, strconv.Quote(b.Name), b.DoneCount, b.TaskCount)
	}
	for _, b := range list.TeamBoards {
		fmt.Fprintf(w, "board %s %s %d/%d done shared\n", b.ID, strconv.Quote(b.Name), b.DoneCount, b.TaskCount)
	}
}

func writeBoard(w io.Writer, b board.Whole) {
	fmt.Fprintf(w, "board %s %s\n", b.ID, strconv.Quote(b.Name))
	if b.Description != "" {
		fmt.Fprintf(w, "  description %s\n", strconv.Quote(b.Description))
	}
	for _, c := range b.Cards {
		fmt.Fprintf(w, "card %s %s\n", c.ID, strconv.Quote(c.Name))
		if c.Description != "" {
			fmt.Fprintf(w, "  description %s\n", strconv.Quote(c.Description))
		}
		for _, t := range c.Tasks {
			writeTask(w, "  ", t.ID, t.Status, t.Name)
			if t.Description != "" {
				fmt.Fprintf(w, "    description %s\n", strconv.Quote(t.Description))
			}
		}
	}
}

func writeTask(w io.Writer, indent, id, status, name string) {
	fmt.Fprintf(w, "%stask %s [%s] %s\n", indent, id, status, strconv.Quote(name))
}
