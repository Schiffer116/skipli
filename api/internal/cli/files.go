package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/boardchange"
	"github.com/Schiffer116/skipli/api/internal/boardfile"
)

// listFiles lists the board files in dir.
func listFiles(e *env, dir string) error {
	found, err := boardfile.List(dir)
	if err != nil {
		return err
	}
	if e.json {
		out := []board.Whole{}
		for _, f := range found {
			out = append(out, f.Board)
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		e.raw(data)
		return nil
	}
	for _, f := range found {
		done, all := 0, 0
		for _, c := range f.Board.Cards {
			for _, t := range c.Tasks {
				all++
				if t.Status == "done" {
					done++
				}
			}
		}
		fmt.Fprintf(e.Stdout, "board %s %s %d/%d done file %s\n", f.Board.ID, strconv.Quote(f.Board.Name), done, all, f.Path)
	}
	return nil
}

func runExport(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	if len(a.pos) > 1 {
		if err := boardfile.Written(a.pos[1], b); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, "board %s %s file %s\n", b.ID, strconv.Quote(b.Name), a.pos[1])
		return nil
	}
	data, err := boardfile.Encode(b)
	if err != nil {
		return err
	}
	_, err = e.Stdout.Write(data)
	return err
}

// runImport makes a new board from a board file: the board, then its cards
// and tasks as Board changes under the file's own ids. A card or task
// description takes a change of its own, as creating one sets none.
func runImport(ctx context.Context, e *env, a *args) error {
	b, err := boardfile.Read(a.pos[0])
	if err != nil {
		return err
	}
	// The board keeps the file's id when it is free, so references to it
	// survive a trip server to file to server; otherwise it gets a new one.
	id := b.ID
	_, err = e.client.call(ctx, "POST", "/api/boards", board.CreateRequest{ID: id, Name: b.Name, Description: b.Description}, nil)
	var api *apiError
	if errors.As(err, &api) && api.Status == http.StatusConflict {
		id = uuid.NewString()
		_, err = e.client.call(ctx, "POST", "/api/boards", board.CreateRequest{ID: id, Name: b.Name, Description: b.Description}, nil)
	}
	if err != nil {
		return err
	}
	changes := func() []boardchange.Change {
		var all []boardchange.Change
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		for _, c := range b.Cards {
			all = append(all, boardchange.Change{Type: boardchange.CreateCard, BoardID: id, CardID: c.ID, Name: new(c.Name), CreatedAt: now})
			if c.Description != "" {
				all = append(all, boardchange.Change{Type: boardchange.UpdateCard, BoardID: id, CardID: c.ID, Description: new(c.Description)})
			}
			for _, t := range c.Tasks {
				all = append(all, boardchange.Change{Type: boardchange.CreateTask, BoardID: id, CardID: c.ID, TaskID: t.ID, Name: new(t.Name)})
				if t.Description != "" || t.Status != "pending" {
					all = append(all, boardchange.Change{Type: boardchange.UpdateTask, BoardID: id, CardID: c.ID, TaskID: t.ID, Description: new(t.Description), Status: new(t.Status)})
				}
			}
		}
		return all
	}()
	for i, c := range changes {
		if _, err := e.client.call(ctx, "POST", "/api/boards/"+id+"/changes", c, changeHeader(uuid.NewString())); err != nil {
			return fmt.Errorf("import stopped at change %d of %d, leaving board %s partly filled: %w", i+1, len(changes), id, err)
		}
	}
	if e.json {
		data, err := json.Marshal(board.Board{ID: id, Name: b.Name, Description: b.Description})
		if err != nil {
			return err
		}
		e.raw(data)
		return nil
	}
	fmt.Fprintf(e.Stdout, "board %s %s\n", id, strconv.Quote(b.Name))
	return nil
}
