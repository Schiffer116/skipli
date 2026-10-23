// Package boardfile reads and writes a board as a file, the skipli-board/1
// format: what belongs to the board itself (its name, description, cards and
// tasks, in order) and nothing that belongs to accounts (owner, members,
// favorite, Presence). It is written the same way every time, so a file
// diffs well in git.
package boardfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"

	"github.com/Schiffer116/skipli/api/internal/board"
)

const Format = "skipli-board/1"

// The structs fix the order of the keys.
type file struct {
	Format      string `json:"format"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cards       []card `json:"cards"`
}

type card struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tasks       []task `json:"tasks"`
}

type task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// IsFile reports whether a board argument names a file: ids and id prefixes
// never contain a slash or end in .json.
func IsFile(ref string) bool {
	return strings.Contains(ref, "/") || strings.HasSuffix(ref, ".json")
}

// Encode writes a board in the file format. Array order is card and task order.
func Encode(b board.Whole) ([]byte, error) {
	f := file{Format: Format, ID: b.ID, Name: b.Name, Description: b.Description, Cards: []card{}}
	for _, c := range b.Cards {
		fc := card{ID: c.ID, Name: c.Name, Description: c.Description, Tasks: []task{}}
		for _, t := range c.Tasks {
			fc.Tasks = append(fc.Tasks, task{ID: t.ID, Name: t.Name, Description: t.Description, Status: t.Status})
		}
		f.Cards = append(f.Cards, fc)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Decode reads the file format. It refuses another format, fields it doesn't
// know (so a typo can't silently vanish), ids that can't be keys, a status
// other than pending or done, and an id used twice.
func Decode(data []byte) (board.Whole, error) {
	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return board.Whole{}, fmt.Errorf("not a skipli board file: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return board.Whole{}, errors.New("not a skipli board file: data after the board")
	}
	if f.Format != Format {
		return board.Whole{}, fmt.Errorf("not a skipli board file: format is %q, want %q", f.Format, Format)
	}

	seen := map[string]bool{}
	check := func(kind, id string) error {
		if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
			return fmt.Errorf("%s id %q is not a lowercase UUID", kind, id)
		}
		if seen[id] {
			return fmt.Errorf("id %s is used twice", id)
		}
		seen[id] = true
		return nil
	}
	if err := check("board", f.ID); err != nil {
		return board.Whole{}, err
	}
	b := board.Whole{ID: f.ID, Name: f.Name, Description: f.Description, Cards: []board.Card{}}
	for _, c := range f.Cards {
		if err := check("card", c.ID); err != nil {
			return board.Whole{}, err
		}
		bc := board.Card{ID: c.ID, Name: c.Name, Description: c.Description, Tasks: []board.Task{}}
		for _, t := range c.Tasks {
			if err := check("task", t.ID); err != nil {
				return board.Whole{}, err
			}
			if t.Status != "pending" && t.Status != "done" {
				return board.Whole{}, fmt.Errorf("task %s has status %q, want pending or done", t.ID, t.Status)
			}
			bc.Tasks = append(bc.Tasks, board.Task{ID: t.ID, CardID: c.ID, Name: t.Name, Description: t.Description, Status: t.Status})
		}
		b.Cards = append(b.Cards, bc)
	}
	return b, nil
}

// Read reads a board file. Writes replace the file whole, so a read never
// sees a half-written one and needs no lock.
func Read(path string) (board.Whole, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return board.Whole{}, err
	}
	b, err := Decode(data)
	if err != nil {
		return board.Whole{}, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// lock holds an exclusive flock on <path>.lock until the returned function runs.
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil // closing releases the lock
}

// write replaces path with b, through a temp file in the same directory.
func write(path string, b board.Whole) error {
	data, err := Encode(b)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	// CreateTemp makes the file 0600; keep the file's own mode, 0644 for a new one.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Create writes a new board file; it refuses to replace one.
func Create(path string, b board.Whole) error {
	unlock, err := lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s: %w", path, os.ErrExist)
	}
	return write(path, b)
}

// Update reads the board file under its lock, lets change edit the board,
// and writes it back whole (a temp file, then a rename), so agents sharing
// one file lose no writes and leave no partial file. When change fails the
// file stays as it was.
func Update(path string, change func(*board.Whole) error) error {
	unlock, err := lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	b, err := Read(path)
	if err != nil {
		return err
	}
	if err := change(&b); err != nil {
		return err
	}
	return write(path, b)
}

// Written replaces the file at path, which may or may not exist, with b.
func Written(path string, b board.Whole) error {
	unlock, err := lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	return write(path, b)
}

// Listed is a board file found in a directory.
type Listed struct {
	Path  string
	Board board.Whole
}

// List finds the board files directly in dir: the .json files that are
// boards, by name order. Other files are skipped.
func List(dir string) ([]Listed, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var found []Listed
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if b, err := Read(path); err == nil {
			found = append(found, Listed{path, b})
		}
	}
	return found, nil
}
