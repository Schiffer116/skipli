package cli_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Schiffer116/skipli/api/internal/cli"
)

// offline is an HTTP client that fails the test: a board file needs no network.
type offline struct{ t *testing.T }

func (o offline) RoundTrip(r *http.Request) (*http.Response, error) {
	o.t.Errorf("board file command called %s", r.URL)
	return nil, errors.New("offline")
}

type fileRun struct {
	t   *testing.T
	dir string
}

func (f fileRun) run(args ...string) (int, string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args: args, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		ConfigDir:  filepath.Join(f.dir, "config"),
		HTTPClient: &http.Client{Transport: offline{f.t}},
	})
	return code, out.String(), errOut.String()
}

func (f fileRun) must(args ...string) string {
	f.t.Helper()
	code, out, errOut := f.run(args...)
	if code != 0 {
		f.t.Fatalf("skipli-cli %v = %d, stderr %q", args, code, errOut)
	}
	return out
}

// id is the second word of the line a command printed ("card <id> ...").
func id(out string) string { return strings.Fields(out)[1] }

func TestEveryBoardCommandWorksOnAFile(t *testing.T) {
	f := fileRun{t, t.TempDir()}
	path := filepath.Join(f.dir, "army.skipli.json")

	out := f.must("new-board", "--file", path, "--description", "the plan", "Army")
	if !strings.HasPrefix(out, "board ") || !strings.HasSuffix(out, ` "Army" file `+path+"\n") {
		t.Fatalf("new-board = %q", out)
	}
	if code, _, errOut := f.run("new-board", "--file", path, "Again"); code != 1 || !strings.Contains(errOut, "exists") {
		t.Errorf("new-board over a file = %d %q", code, errOut)
	}
	boardID := id(out)

	backlog := id(f.must("add-card", path, "Backlog"))
	doing := id(f.must("add-card", path, "Doing"))
	write := id(f.must("add-task", path, backlog[:4], "Write"))
	ship := id(f.must("add-task", path, backlog, "Ship"))
	review := id(f.must("add-task", path, doing, "Review"))
	f.must("edit-card", path, backlog, "--description", "Notes\nline 2")
	f.must("edit-task", path, ship, "--status", "done", "--description", "d")
	f.must("move-task", path, review, "--card", backlog, "--before", ship)
	f.must("move-card", path, doing, "--before", backlog)
	f.must("move-task", path, write, "--after", ship)

	want := func(extra string) string {
		return `board ` + boardID + ` "Army"
  description "the plan"
card ` + doing + ` "Doing"
card ` + backlog + ` "Backlog"
  description "Notes\nline 2"
` + extra
	}
	if got := f.must("board", path); got != want(`  task `+review+` [pending] "Review"
  task `+ship+` [done] "Ship"
    description "d"
  task `+write+` [pending] "Write"
`) {
		t.Errorf("board =\n%s", got)
	}
	if got := f.must("board", path, "--json"); !strings.Contains(got, `"name":"Army"`) || !strings.Contains(got, `"status":"done"`) {
		t.Errorf("board --json = %s", got)
	}

	if got := f.must("clean-up", path); got != "removed 1 done tasks\n" {
		t.Errorf("clean-up = %q", got)
	}
	f.must("delete-task", path, review)
	f.must("delete-card", path, doing)
	if got := f.must("board", path); got != `board `+boardID+` "Army"
  description "the plan"
card `+backlog+` "Backlog"
  description "Notes\nline 2"
  task `+write+` [pending] "Write"
` {
		t.Errorf("board after deletes =\n%s", got)
	}

	if got := f.must("boards", f.dir); got != "board "+boardID+` "Army" 0/1 done file `+path+"\n" {
		t.Errorf("boards dir = %q", got)
	}
}

func TestAFileBoardRefusesWhatTheServerWould(t *testing.T) {
	f := fileRun{t, t.TempDir()}
	path := filepath.Join(f.dir, "a.json")
	f.must("new-board", "--file", path, "A")
	card := id(f.must("add-card", path, "Todo"))

	before, _ := os.ReadFile(path)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"add-task", path, "zzzz", "x"}, `no card with id "zzzz"`},
		{[]string{"move-card", path, card, "--before", card}, "no card"},
		{[]string{"board", filepath.Join(f.dir, "nope.json")}, "no such file"},
	} {
		code, _, errOut := f.run(c.args...)
		if code == 0 || !strings.Contains(errOut, c.want) {
			t.Errorf("%v = %d %q, want %q", c.args, code, errOut, c.want)
		}
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("a refused command changed the file")
	}
}

func TestExportCopiesAFileExactly(t *testing.T) {
	f := fileRun{t, t.TempDir()}
	src, dst := filepath.Join(f.dir, "a.json"), filepath.Join(f.dir, "b.json")
	f.must("new-board", "--file", src, "Army")
	card := id(f.must("add-card", src, "Notes <&>"))
	f.must("edit-card", src, card, "--description", "# Heading\n\n- one\n\ttab \"quoted\"")
	f.must("add-task", src, card, "Task")

	f.must("export", src, dst)
	a, _ := os.ReadFile(src)
	b, _ := os.ReadFile(dst)
	if !bytes.Equal(a, b) {
		t.Errorf("copy differs:\n%s\n%s", a, b)
	}
	if out := f.must("export", src); out != string(a) {
		t.Errorf("export to stdout differs:\n%s", out)
	}
	if !strings.HasSuffix(string(a), "}\n") || !strings.Contains(string(a), `"format": "skipli-board/1"`) {
		t.Errorf("file = %s", a)
	}
}

func TestParallelAgentsOnOneFileLoseNothing(t *testing.T) {
	f := fileRun{t, t.TempDir()}
	path := filepath.Join(f.dir, "a.json")
	f.must("new-board", "--file", path, "A")
	done := make(chan struct{})
	const agents = 12
	for i := 0; i < agents; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			var out, errOut bytes.Buffer
			code := cli.Run(context.Background(), cli.Options{
				Args: []string{"add-card", path, "card"}, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
				ConfigDir: filepath.Join(f.dir, "config"),
			})
			if code != 0 {
				t.Errorf("add-card = %d %q", code, errOut.String())
			}
		}()
	}
	for i := 0; i < agents; i++ {
		<-done
	}
	if got := strings.Count(f.must("board", path), "card "); got != agents {
		t.Errorf("%d cards, want %d", got, agents)
	}
}
