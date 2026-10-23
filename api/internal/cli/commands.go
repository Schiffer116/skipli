package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Schiffer116/skipli/api/internal/board"
	"github.com/Schiffer116/skipli/api/internal/boardchange"
	"github.com/Schiffer116/skipli/api/internal/boardfile"
)

func findCommand(name string) *command {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

var commands = []command{
	{
		name: "login", args: []string{"email"},
		help: `Sign in. Emails a code to <email>. At a terminal it asks for the code
and finishes; otherwise it saves the sign-in and exits, and you finish it with
login --code <code>. The code is mailed at most once a minute per address.`,
		flags: func(fs *flag.FlagSet) { fs.String("code", "", "finish a sign-in started by login <email>") },
		run:   runLogin,
	},
	{
		name: "logout",
		help: "Sign out: revokes this Session on the server and deletes the stored one.",
		run:  runLogout,
	},
	{
		name: "boards", args: []string{"dir"},
		help: `List your boards, one per line: board <id> "name" <done>/<all> done, with
"shared" after boards you are only a member of.
With a directory, list the board files in it instead (no sign-in needed):
board <id> "name" <done>/<all> done file <path>.`,
		run: runBoards,
	},
	{
		name: "board", args: []string{"board"}, min: 1,
		help: `Show a board: its cards in order, each with its tasks in order.
  card <id> "name"
    task <id> [pending|done] "name"
Descriptions, when set, follow on a "description" line.`,
		run: runBoard,
	},
	{
		name: "new-board", args: []string{"name"}, min: 1,
		help: `Create a board you own. With --file, create a board file instead (no sign-in
needed); it refuses to replace a file that exists.`,
		flags: func(fs *flag.FlagSet) {
			fs.String("description", "", "the board's description")
			fs.String("file", "", "create a board file at this path")
		},
		run: runNewBoard,
	},
	{
		name: "export", args: []string{"board", "file"}, min: 1,
		help: `Write a board in the skipli-board/1 format (JSON; cards and tasks in order,
with descriptions) to <file>, or to stdout without one. <board> may be a
board file, which makes this a copy. A file is replaced if it exists.`,
		run: runExport,
	},
	{
		name: "import", args: []string{"file"}, min: 1,
		help: `Make a new board you own from a skipli-board/1 file and print its id. The
board keeps the file's id if no board has it (so moving a board from the server
to a file and back keeps references to it working), else gets a new one. Its
cards and tasks keep the file's ids and order, and descriptions come along.`,
		run: runImport,
	},
	{
		name: "add-card", args: []string{"board", "name"}, min: 2,
		help: "Add a card at the end of the board. Prints the new card.",
		run:  runAddCard,
	},
	{
		name: "edit-card", args: []string{"board", "card"}, min: 2,
		help: "Change a card's name or description; what you don't give stays as it is.",
		flags: func(fs *flag.FlagSet) {
			fs.String("name", "", "new name")
			fs.String("description", "", "new description")
		},
		run: runEditCard,
	},
	{
		name: "move-card", args: []string{"board", "card"}, min: 2,
		help: "Move a card within the board's card order. Goes to the end unless --before or --after names another card.",
		flags: func(fs *flag.FlagSet) {
			fs.String("before", "", "put the card right before this card")
			fs.String("after", "", "put the card right after this card")
		},
		run: runMoveCard,
	},
	{
		name: "delete-card", args: []string{"board", "card"}, min: 2,
		help: "Delete a card and all its tasks.",
		run:  runDeleteCard,
	},
	{
		name: "add-task", args: []string{"board", "card", "name"}, min: 3,
		help: "Add a pending task at the end of a card. Prints the new task.",
		run:  runAddTask,
	},
	{
		name: "edit-task", args: []string{"board", "task"}, min: 2,
		help: "Change a task's name, description or status; what you don't give stays as it is.",
		flags: func(fs *flag.FlagSet) {
			fs.String("name", "", "new name")
			fs.String("description", "", "new description")
			fs.String("status", "", "pending or done")
		},
		run: runEditTask,
	},
	{
		name: "move-task", args: []string{"board", "task"}, min: 2,
		help: `Move a task within its card or to another card. Goes to the end of the card
unless --before or --after names another task of that card.`,
		flags: func(fs *flag.FlagSet) {
			fs.String("card", "", "move to this card (default: stay in the task's card)")
			fs.String("before", "", "put the task right before this task")
			fs.String("after", "", "put the task right after this task")
		},
		run: runMoveTask,
	},
	{
		name: "delete-task", args: []string{"board", "task"}, min: 2,
		help: "Delete a task.",
		run:  runDeleteTask,
	},
	{
		name: "clean-up", args: []string{"board"}, min: 1,
		help: "Delete every done task on the board.",
		run:  runCleanUp,
	},
}

// ---- sign-in ----

func runLogin(ctx context.Context, e *env, a *args) error {
	c := e.client
	if code := a.str("code"); code != "" {
		if len(a.pos) > 0 {
			return usageError{"login takes an email or --code, not both"}
		}
		if c.sess.Challenge == nil {
			return errors.New("no sign-in waiting for a code: run `" + e.follow() + " login <email>` first")
		}
		return e.verify(ctx, *c.sess.Challenge, code)
	}
	if len(a.pos) == 0 {
		return usageError{"usage: login <email>, or login --code <code>"}
	}

	email := strings.ToLower(strings.TrimSpace(a.pos[0]))
	_, data, err := c.send(ctx, "POST", "/api/auth/send", map[string]string{"email": email}, nil)
	if err != nil {
		return err
	}
	var sent struct{ Session string }
	if err := json.Unmarshal(data, &sent); err != nil || sent.Session == "" {
		return errors.New("the server sent no sign-in session")
	}
	ch := challenge{Email: email, Session: sent.Session}

	if !e.Interactive {
		c.sess.Challenge = &ch
		if err := saveSession(c.path, c.sess); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, "code sent to %s; finish with: %s login --code <code>\n", email, e.follow())
		return nil
	}
	fmt.Fprint(e.Stderr, "Code: ")
	line, err := bufio.NewReader(e.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("no code entered")
	}
	return e.verify(ctx, ch, strings.TrimSpace(line))
}

func (e *env) verify(ctx context.Context, ch challenge, code string) error {
	c := e.client
	resp, _, err := c.send(ctx, "POST", "/api/auth/verify", map[string]string{
		"email": ch.Email, "code": code, "session": ch.Session,
	}, nil)
	if err != nil {
		return err
	}
	id, refresh := cookie(resp, "id_token"), cookie(resp, "refresh_token")
	if id == "" || refresh == "" {
		return errors.New("the server sent no tokens")
	}
	c.sess = session{IDToken: id, RefreshToken: refresh}
	if err := saveSession(c.path, c.sess); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "signed in as %s\n", ch.Email)
	return nil
}

func runLogout(ctx context.Context, e *env, a *args) error {
	c := e.client
	if c.sess.RefreshToken != "" {
		_, _, err := c.send(ctx, "POST", "/api/auth/signout", nil, nil, &http.Cookie{Name: "refresh_token", Value: c.sess.RefreshToken})
		if err != nil {
			fmt.Fprintf(e.Stderr, "skipli-cli: the server did not confirm the sign-out: %v\n", err)
		}
	}
	if err := deleteSession(c.path); err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, "signed out")
	return nil
}

// ---- reading ----

func (e *env) get(ctx context.Context, path string, into any) ([]byte, error) {
	data, err := e.client.call(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	return data, json.Unmarshal(data, into)
}

func (e *env) raw(data []byte) {
	fmt.Fprintln(e.Stdout, strings.TrimSpace(string(data)))
}

func runBoards(ctx context.Context, e *env, a *args) error {
	if len(a.pos) > 0 {
		return listFiles(e, a.pos[0])
	}
	var list board.ListResponse
	data, err := e.get(ctx, "/api/boards", &list)
	if err != nil {
		return err
	}
	if e.json {
		e.raw(data)
		return nil
	}
	writeBoards(e.Stdout, list)
	return nil
}

// boardID resolves a board id or unique prefix of one.
func (e *env) boardID(ctx context.Context, ref string) (string, error) {
	if boardfile.IsFile(ref) {
		b, err := boardfile.Read(ref)
		if err != nil {
			return "", err
		}
		if e.files == nil {
			e.files = map[string]string{}
		}
		e.files[b.ID] = ref
		return b.ID, nil
	}
	if isUUID(ref) {
		return ref, nil
	}
	var list board.ListResponse
	if _, err := e.get(ctx, "/api/boards", &list); err != nil {
		return "", err
	}
	var ids []string
	for _, b := range append(list.Boards, list.TeamBoards...) {
		ids = append(ids, b.ID)
	}
	return pick("board", ref, ids)
}

// readBoard resolves ref and reads that whole board.
func (e *env) readBoard(ctx context.Context, ref string) (board.Whole, []byte, error) {
	id, err := e.boardID(ctx, ref)
	if err != nil {
		return board.Whole{}, nil, err
	}
	if path, ok := e.files[id]; ok {
		whole, err := boardfile.Read(path)
		if err != nil {
			return board.Whole{}, nil, err
		}
		whole.Members = []board.Member{}
		data, err := json.Marshal(whole)
		return whole, data, err
	}
	var whole board.Whole
	data, err := e.get(ctx, "/api/boards/"+id, &whole)
	return whole, data, err
}

func runBoard(ctx context.Context, e *env, a *args) error {
	whole, data, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	if e.json {
		e.raw(data)
		return nil
	}
	writeBoard(e.Stdout, whole)
	return nil
}

// ---- writing ----

func runNewBoard(ctx context.Context, e *env, a *args) error {
	if path := a.str("file"); path != "" {
		b := board.Whole{ID: uuid.NewString(), Name: a.pos[0], Description: a.str("description"), Cards: []board.Card{}}
		if err := boardfile.Create(path, b); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, "board %s %s file %s\n", b.ID, strconv.Quote(b.Name), path)
		return nil
	}
	data, err := e.client.call(ctx, "POST", "/api/boards", board.CreateRequest{
		ID: uuid.NewString(), Name: a.pos[0], Description: a.str("description"),
	}, nil)
	if err != nil {
		return err
	}
	if e.json {
		e.raw(data)
		return nil
	}
	var b board.Board
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "board %s %s\n", b.ID, strconv.Quote(b.Name))
	return nil
}

// send makes one Board change. With --json it prints the change as sent;
// otherwise the caller prints what it made.
func (e *env) send(ctx context.Context, c boardchange.Change) error {
	if path, ok := e.files[c.BoardID]; ok {
		err := boardfile.Update(path, func(b *board.Whole) error { return boardchange.Reduce(b, c) })
		if err != nil {
			return err
		}
	} else if _, err := e.client.call(ctx, "POST", "/api/boards/"+c.BoardID+"/changes", c, changeHeader(uuid.NewString())); err != nil {
		return err
	}
	if e.json {
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		e.raw(data)
	}
	return nil
}

func (e *env) ok() {
	if !e.json {
		fmt.Fprintln(e.Stdout, "ok")
	}
}

func cardIDs(b board.Whole) []string {
	var ids []string
	for _, c := range b.Cards {
		ids = append(ids, c.ID)
	}
	return ids
}

func (e *env) card(b board.Whole, ref string) (board.Card, error) {
	id, err := pick("card", ref, cardIDs(b))
	if err != nil {
		return board.Card{}, err
	}
	for _, c := range b.Cards {
		if c.ID == id {
			return c, nil
		}
	}
	panic("unreachable")
}

// task finds a task on any card of the board.
func (e *env) task(b board.Whole, ref string) (board.Task, error) {
	var ids []string
	for _, c := range b.Cards {
		for _, t := range c.Tasks {
			ids = append(ids, t.ID)
		}
	}
	id, err := pick("task", ref, ids)
	if err != nil {
		return board.Task{}, err
	}
	for _, c := range b.Cards {
		for _, t := range c.Tasks {
			if t.ID == id {
				return t, nil
			}
		}
	}
	panic("unreachable")
}

func runAddCard(ctx context.Context, e *env, a *args) error {
	id, err := e.boardID(ctx, a.pos[0])
	if err != nil {
		return err
	}
	c := boardchange.Change{
		Type: boardchange.CreateCard, BoardID: id, CardID: uuid.NewString(), Name: new(a.pos[1]),
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if err := e.send(ctx, c); err != nil {
		return err
	}
	if !e.json {
		fmt.Fprintf(e.Stdout, "card %s %s\n", c.CardID, strconv.Quote(a.pos[1]))
	}
	return nil
}

func runEditCard(ctx context.Context, e *env, a *args) error {
	if !a.set("name") && !a.set("description") {
		return usageError{"edit-card: give --name or --description"}
	}
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	card, err := e.card(b, a.pos[1])
	if err != nil {
		return err
	}
	c := boardchange.Change{Type: boardchange.UpdateCard, BoardID: b.ID, CardID: card.ID}
	if a.set("name") {
		c.Name = new(a.str("name"))
	}
	if a.set("description") {
		c.Description = new(a.str("description"))
	}
	if err := e.send(ctx, c); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runMoveCard(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	card, err := e.card(b, a.pos[1])
	if err != nil {
		return err
	}
	var siblings []string
	for _, id := range cardIDs(b) {
		if id != card.ID {
			siblings = append(siblings, id)
		}
	}
	before, after, err := place("card", a.str("before"), a.str("after"), siblings)
	if err != nil {
		return err
	}
	if err := e.send(ctx, boardchange.Change{Type: boardchange.MoveCard, BoardID: b.ID, CardID: card.ID, BeforeID: before, AfterID: after}); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runDeleteCard(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	card, err := e.card(b, a.pos[1])
	if err != nil {
		return err
	}
	if err := e.send(ctx, boardchange.Change{Type: boardchange.DeleteCard, BoardID: b.ID, CardID: card.ID}); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runAddTask(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	card, err := e.card(b, a.pos[1])
	if err != nil {
		return err
	}
	c := boardchange.Change{Type: boardchange.CreateTask, BoardID: b.ID, CardID: card.ID, TaskID: uuid.NewString(), Name: new(a.pos[2])}
	if err := e.send(ctx, c); err != nil {
		return err
	}
	if !e.json {
		writeTask(e.Stdout, "", c.TaskID, "pending", a.pos[2])
	}
	return nil
}

func runEditTask(ctx context.Context, e *env, a *args) error {
	if !a.set("name") && !a.set("description") && !a.set("status") {
		return usageError{"edit-task: give --name, --description or --status"}
	}
	if a.set("status") && a.str("status") != "pending" && a.str("status") != "done" {
		return usageError{"edit-task: --status is pending or done"}
	}
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	task, err := e.task(b, a.pos[1])
	if err != nil {
		return err
	}
	c := boardchange.Change{
		Type: boardchange.UpdateTask, BoardID: b.ID, CardID: task.CardID, TaskID: task.ID,
	}
	if a.set("name") {
		c.Name = new(a.str("name"))
	}
	if a.set("description") {
		c.Description = new(a.str("description"))
	}
	if a.set("status") {
		c.Status = new(a.str("status"))
	}
	if err := e.send(ctx, c); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runMoveTask(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	task, err := e.task(b, a.pos[1])
	if err != nil {
		return err
	}
	target := task.CardID
	if a.set("card") {
		card, err := e.card(b, a.str("card"))
		if err != nil {
			return err
		}
		target = card.ID
	}
	var siblings []string
	for _, card := range b.Cards {
		if card.ID != target {
			continue
		}
		for _, t := range card.Tasks {
			if t.ID != task.ID {
				siblings = append(siblings, t.ID)
			}
		}
	}
	before, after, err := place("task", a.str("before"), a.str("after"), siblings)
	if err != nil {
		return err
	}
	if err := e.send(ctx, boardchange.Change{
		Type: boardchange.MoveTask, BoardID: b.ID, CardID: task.CardID, TaskID: task.ID,
		NewCardID: target, BeforeID: before, AfterID: after,
	}); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runDeleteTask(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	task, err := e.task(b, a.pos[1])
	if err != nil {
		return err
	}
	if err := e.send(ctx, boardchange.Change{Type: boardchange.DeleteTask, BoardID: b.ID, CardID: task.CardID, TaskID: task.ID}); err != nil {
		return err
	}
	e.ok()
	return nil
}

func runCleanUp(ctx context.Context, e *env, a *args) error {
	b, _, err := e.readBoard(ctx, a.pos[0])
	if err != nil {
		return err
	}
	done := []string{}
	for _, card := range b.Cards {
		for _, t := range card.Tasks {
			if t.Status == "done" {
				done = append(done, t.ID)
			}
		}
	}
	if err := e.send(ctx, boardchange.Change{Type: boardchange.CleanUp, BoardID: b.ID, TaskIDs: done}); err != nil {
		return err
	}
	if !e.json {
		fmt.Fprintf(e.Stdout, "removed %d done tasks\n", len(done))
	}
	return nil
}
