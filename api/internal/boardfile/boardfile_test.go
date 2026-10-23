package boardfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/boardchange"
)

const (
	bid = "b0000000-0000-4000-8000-000000000001"
	cid = "c0000000-0000-4000-8000-000000000001"
	tid = "d0000000-0000-4000-8000-000000000001"
)

func sample() board.Whole {
	return board.Whole{ID: bid, Name: "Army <&> \"notes\"", Description: "line 1\nline 2", Cards: []board.Card{
		{ID: cid, Name: "Backlog", Description: "# Note\n\ttabbed", Tasks: []board.Task{
			{ID: tid, CardID: cid, Name: "Ship", Description: "", Status: "done"},
		}},
		{ID: "c0000000-0000-4000-8000-000000000002", Name: "Empty", Tasks: []board.Task{}},
	}}
}

const golden = `{
  "format": "skipli-board/1",
  "id": "b0000000-0000-4000-8000-000000000001",
  "name": "Army <&> \"notes\"",
  "description": "line 1\nline 2",
  "cards": [
    {
      "id": "c0000000-0000-4000-8000-000000000001",
      "name": "Backlog",
      "description": "# Note\n\ttabbed",
      "tasks": [
        {
          "id": "d0000000-0000-4000-8000-000000000001",
          "name": "Ship",
          "description": "",
          "status": "done"
        }
      ]
    },
    {
      "id": "c0000000-0000-4000-8000-000000000002",
      "name": "Empty",
      "description": "",
      "tasks": []
    }
  ]
}
`

func TestEncodeIsStableAndReadable(t *testing.T) {
	got, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != golden {
		t.Errorf("got\n%s\nwant\n%s", got, golden)
	}
}

func TestDecodeGivesBackWhatWasEncoded(t *testing.T) {
	got, err := Decode([]byte(golden))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, sample()) {
		t.Errorf("got %+v, want %+v", got, sample())
	}
	again, _ := Encode(got)
	if string(again) != golden {
		t.Error("encoding what was decoded changed it")
	}
}

func TestDecodeRefusesWhatItDoesNotKnow(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(golden, old, new, 1) }
	for name, in := range map[string]string{
		"another format":    edit("skipli-board/1", "skipli-board/2"),
		"no format":         edit(`"format": "skipli-board/1",`, ""),
		"an unknown field":  edit(`"name": "Backlog",`, `"name": "Backlog", "colour": "red",`),
		"a typo":            edit(`"description": "line 1\nline 2"`, `"descripton": "line 1\nline 2"`),
		"a bad status":      edit(`"done"`, `"finished"`),
		"a bad board id":    edit(bid, "b1"),
		"a bad card id":     edit(cid, "UPPER-CASE"),
		"a duplicate card":  edit("c0000000-0000-4000-8000-000000000002", cid),
		"a duplicate task":  edit(`"tasks": []`, `"tasks": [{"id": "`+tid+`", "name": "x", "description": "", "status": "pending"}]`),
		"trailing data":     golden + "{}",
		"not json":          "cards:\n",
		"a number for text": edit(`"Backlog"`, "7"),
	} {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("%s: decoded without an error", name)
		}
	}
}

func TestReadAndCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.skipli.json")
	if _, err := Read(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Read of nothing = %v", err)
	}
	if err := Create(path, sample()); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || !reflect.DeepEqual(got, sample()) {
		t.Errorf("Read = %+v, %v", got, err)
	}
	if err := Create(path, sample()); !errors.Is(err, os.ErrExist) {
		t.Errorf("Create over a file = %v, want it to refuse", err)
	}
}

func TestReadNamesTheFileWhenItIsNotABoard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(path, []byte(`{"a":1}`), 0o644)
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("Read = %v, want the path in the error", err)
	}
}

func TestUpdateKeepsEveryChangeFromParallelWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.skipli.json")
	if err := Create(path, sample()); err != nil {
		t.Fatal(err)
	}
	const writers = 20
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := Update(path, func(b *board.Whole) error {
				return boardchange.Reduce(b, boardchange.Change{
					Type: boardchange.CreateCard, BoardID: bid, CardID: uuidN(i), Name: "n",
				})
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != 2+writers {
		t.Errorf("%d cards, want %d: writes were lost", len(got.Cards), 2+writers)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func uuidN(i int) string {
	return strings.Replace("e0000000-0000-4000-8000-0000000000NN", "NN", string(rune('0'+i/10))+string(rune('0'+i%10)), 1)
}

func TestUpdateThatFailsWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.skipli.json")
	Create(path, sample())
	before, _ := os.ReadFile(path)
	boom := errors.New("boom")
	if err := Update(path, func(b *board.Whole) error { b.Name = "changed"; return boom }); !errors.Is(err, boom) {
		t.Fatalf("Update = %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("the file changed")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	Create(filepath.Join(dir, "a.json"), sample())
	os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{"a":1}`), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`x`), 0o644)
	os.Mkdir(filepath.Join(dir, "sub.json"), 0o755)
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != filepath.Join(dir, "a.json") || got[0].Board.ID != bid {
		t.Errorf("List = %+v", got)
	}
}

func TestIsFile(t *testing.T) {
	for ref, want := range map[string]bool{
		"./a": true, "dir/a": true, "a.json": true, "a.skipli.json": true, "/abs": true,
		"3f2a": false, bid: false, "": false,
	} {
		if IsFile(ref) != want {
			t.Errorf("IsFile(%q) = %v", ref, !want)
		}
	}
}
