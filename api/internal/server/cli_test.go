package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/awsfake"
	"github.com/Schiffer116/skipli/api/internal/cli"
)

const (
	cliBoard = "b0000000-0000-4000-8000-000000000001"
	cliCard1 = "aaaa1111-0000-4000-8000-000000000001"
	cliCard2 = "aaaa2222-0000-4000-8000-000000000002"
	cliTask1 = "dddd1111-0000-4000-8000-000000000001"
	cliTask2 = "dddd2222-0000-4000-8000-000000000002"
	cliTask3 = "eeee3333-0000-4000-8000-000000000003"
	cliCode  = "12345678"
)

// cliHarness runs skipli-cli against the real router, with DynamoDB and
// Cognito faked: reads come from a fixed board, writes are recorded.
type cliHarness struct {
	t      *testing.T
	url    string
	dir    string
	key    *rsa.PrivateKey
	fake   *awsfake.Server
	cooled bool // the send cooldown is running

	mu      sync.Mutex
	changes []map[string]any // bodies of accepted POST .../changes
	headers []string         // their X-Change-Id
}

func item(fields map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range fields {
		switch v := v.(type) {
		case string:
			out[k] = map[string]any{"S": v}
		case int:
			out[k] = map[string]any{"N": itoa(v)}
		}
	}
	return out
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newCLIHarness(t *testing.T) *cliHarness {
	h := &cliHarness{t: t, dir: t.TempDir()}
	var err error
	if h.key, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}

	// Writes are only recorded, so the board reads the same in every test.
	items := map[string]map[string]any{}
	table := awsfake.NewTable()
	add := func(f map[string]any) {
		items[f["PK"].(string)+"/"+f["SK"].(string)] = item(f)
		table.Put(item(f))
	}
	add(map[string]any{"PK": cliBoard, "SK": "META", "ID": cliBoard, "Owner": "u-member", "Name": "Roadmap", "Description": "plans"})
	add(map[string]any{"PK": cliBoard, "SK": "MEMBER#u-member", "Member": "u-member", "Email": "u-member@example.com", "Order": 1})
	add(map[string]any{"PK": cliBoard, "SK": "CARD#" + cliCard1, "ID": cliCard1, "Name": "Backlog", "Description": "later", "Order": 1000})
	add(map[string]any{"PK": cliBoard, "SK": "CARD#" + cliCard2, "ID": cliCard2, "Name": "Doing", "Description": "", "Order": 2000})
	add(map[string]any{"PK": cliBoard, "SK": "TASK#" + cliCard1 + "#" + cliTask1, "ID": cliTask1, "CardID": cliCard1, "Name": "Write tests", "Description": "all of them", "Status": "pending", "Order": 1000})
	add(map[string]any{"PK": cliBoard, "SK": "TASK#" + cliCard1 + "#" + cliTask2, "ID": cliTask2, "CardID": cliCard1, "Name": "Ship", "Description": "", "Status": "done", "Order": 2000})
	add(map[string]any{"PK": cliBoard, "SK": "TASK#" + cliCard2 + "#" + cliTask3, "ID": cliTask3, "CardID": cliCard2, "Name": "Review", "Description": "", "Status": "pending", "Order": 1000})

	str := func(m any, path ...string) string {
		for _, p := range path {
			mm, _ := m.(map[string]any)
			m = mm[p]
		}
		s, _ := m.(string)
		return s
	}
	h.fake = awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "GetItem":
			key := str(body, "Key", "PK", "S") + "/" + str(body, "Key", "SK", "S")
			if it, ok := items[key]; ok {
				return 200, map[string]any{"Item": it}
			}
			if sk := str(body, "Key", "SK", "S"); sk == "MEMBER#u-member" { // of any board, such as one just made
				return 200, map[string]any{"Item": item(map[string]any{"PK": str(body, "Key", "PK", "S"), "SK": sk, "Member": "u-member"})}
			}
			return 200, map[string]any{}
		case "Query":
			if body["IndexName"] != nil {
				return 200, map[string]any{"Items": []any{map[string]any{"PK": map[string]string{"S": cliBoard}}}}
			}
			return table.Respond(op, body)
		case "BatchGetItem":
			var found []any
			keys, _ := body["RequestItems"].(map[string]any)["Skipli"].(map[string]any)["Keys"].([]any)
			for _, k := range keys {
				if it, ok := items[str(k, "PK", "S")+"/"+str(k, "SK", "S")]; ok {
					found = append(found, it)
				}
			}
			return 200, map[string]any{"Responses": map[string]any{"Skipli": found}}
		case "PutItem":
			if str(body, "Item", "SK", "S") == "SEND_COOLDOWN" && h.cooled {
				return 400, map[string]any{"__type": "com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException", "message": "running"}
			}
			return 200, map[string]any{}
		case "AdminCreateUser":
			return 200, map[string]any{"User": map[string]any{"Username": "u"}}
		case "InitiateAuth":
			if str(body, "AuthFlow") == "REFRESH_TOKEN_AUTH" {
				if str(body, "AuthParameters", "REFRESH_TOKEN") != "refresh-1" {
					return 400, map[string]any{"__type": "NotAuthorizedException", "message": "revoked"}
				}
				return 200, map[string]any{"AuthenticationResult": map[string]any{"IdToken": h.token(h.key)}}
			}
			return 200, map[string]any{"Session": "challenge-1"}
		case "RespondToAuthChallenge":
			if str(body, "ChallengeResponses", "EMAIL_OTP_CODE") != cliCode {
				return 400, map[string]any{"__type": "CodeMismatchException", "message": "wrong"}
			}
			return 200, map[string]any{"AuthenticationResult": map[string]any{"IdToken": h.token(h.key), "RefreshToken": "refresh-1"}}
		}
		return 200, map[string]any{}
	})

	tokens := auth.NewTokenVerifierWithKeyfunc(func(*jwt.Token) (any, error) { return &h.key.PublicKey, nil }, testIssuer, testClientID)
	broadcasts := &recordedBroadcasts{}
	router := newRouter(h.fake.DynamoDB(), h.fake.Cognito(), tokens, "us-east-1_test", testClientID, "Skipli",
		func(context.Context, string) bool { return true }, broadcasts, broadcasts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/changes") {
			data, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(data))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, r)
			if rec.Code == 204 {
				var change map[string]any
				json.Unmarshal(data, &change)
				h.mu.Lock()
				h.changes = append(h.changes, change)
				h.headers = append(h.headers, r.Header.Get("X-Change-Id"))
				h.mu.Unlock()
			}
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			w.Write(rec.Body.Bytes())
			return
		}
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	h.url = srv.URL
	return h
}

func (h *cliHarness) token(key *rsa.PrivateKey) string { return signToken(h.t, key, "u-member") }

// run runs skipli-cli with args and stdin, returning its status, stdout and stderr.
func (h *cliHarness) run(stdin string, args ...string) (int, string, string) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:        args,
		Stdin:       strings.NewReader(stdin),
		Stdout:      &out,
		Stderr:      &errOut,
		Interactive: stdin != "",
		Getenv:      func(k string) string { return map[string]string{"SKIPLI_URL": h.url}[k] },
		ConfigDir:   h.dir,
	})
	return code, out.String(), errOut.String()
}

func (h *cliHarness) mustRun(args ...string) string {
	h.t.Helper()
	code, out, errOut := h.run("", args...)
	if code != 0 {
		h.t.Fatalf("skipli-cli %v = %d, stderr %q", args, code, errOut)
	}
	return out
}

func (h *cliHarness) signIn() {
	h.t.Helper()
	h.mustRun("login", "agent@example.com")
	h.mustRun("login", "--code", cliCode)
}

func (h *cliHarness) sessionPath() string {
	files, _ := filepath.Glob(filepath.Join(h.dir, "*.json"))
	if len(files) != 1 {
		h.t.Fatalf("session files = %v, want one", files)
	}
	return files[0]
}

func (h *cliHarness) takeChanges() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, header := range h.headers {
		if header == "" {
			h.t.Error("change sent without an X-Change-Id")
		}
	}
	c := h.changes
	h.changes, h.headers = nil, nil
	return c
}

func TestCLILogin(t *testing.T) {
	t.Run("without a terminal it finishes with --code", func(t *testing.T) {
		h := newCLIHarness(t)
		out := h.mustRun("login", "Agent@Example.com")
		if !strings.Contains(out, "login --code") {
			t.Errorf("login said %q, want the follow-up command", out)
		}
		if code, _, errOut := h.run("", "boards"); code == 0 || !strings.Contains(errOut, "not signed in") {
			t.Errorf("boards before the code = %d %q", code, errOut)
		}
		if out := h.mustRun("login", "--code", cliCode); !strings.Contains(out, "agent@example.com") {
			t.Errorf("--code said %q", out)
		}
		info, err := os.Stat(h.sessionPath())
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("session file = %v, %v, want mode 0600", info, err)
		}
		if out := h.mustRun("boards"); !strings.Contains(out, `board `+cliBoard+` "Roadmap" 1/3 done`) {
			t.Errorf("boards = %q", out)
		}
	})

	t.Run("at a terminal it asks for the code", func(t *testing.T) {
		h := newCLIHarness(t)
		code, out, errOut := h.run(cliCode+"\n", "login", "agent@example.com")
		if code != 0 || !strings.Contains(errOut, "Code:") || !strings.Contains(out, "signed in") {
			t.Errorf("login = %d %q %q", code, out, errOut)
		}
		h.mustRun("boards")
	})

	t.Run("a wrong code or a cooldown is shown as the server says it", func(t *testing.T) {
		h := newCLIHarness(t)
		if code, _, errOut := h.run("0000\n", "login", "agent@example.com"); code != 1 || !strings.Contains(errOut, "invalid email or verification code") {
			t.Errorf("wrong code = %d %q", code, errOut)
		}
		h.cooled = true
		if code, _, errOut := h.run("", "login", "agent@example.com"); code != 1 || !strings.Contains(errOut, "wait before requesting another") {
			t.Errorf("cooldown = %d %q", code, errOut)
		}
	})

	t.Run("logout revokes the Session and forgets it", func(t *testing.T) {
		h := newCLIHarness(t)
		h.signIn()
		path := h.sessionPath()
		h.mustRun("logout")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("session file after logout: %v", err)
		}
		if len(h.fake.Calls("RevokeToken")) != 1 {
			t.Errorf("RevokeToken calls = %d, want 1", len(h.fake.Calls("RevokeToken")))
		}
	})
}

func TestCLIRefresh(t *testing.T) {
	setTokens := func(t *testing.T, h *cliHarness, id, refresh string) {
		data, _ := json.Marshal(map[string]string{"idToken": id, "refreshToken": refresh})
		if err := os.WriteFile(h.sessionPath(), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a rejected id token is refreshed once", func(t *testing.T) {
		h := newCLIHarness(t)
		h.signIn()
		other, _ := rsa.GenerateKey(rand.Reader, 2048)
		bad := h.token(other)
		setTokens(t, h, bad, "refresh-1")
		h.mustRun("boards")
		if n := len(h.fake.Calls("InitiateAuth")); n != 2 { // sign-in start, then one refresh
			t.Errorf("InitiateAuth calls = %d, want 2", n)
		}
		var stored struct{ IDToken string }
		data, _ := os.ReadFile(h.sessionPath())
		json.Unmarshal(data, &stored)
		if stored.IDToken == "" || stored.IDToken == bad {
			t.Errorf("stored id token not replaced")
		}
		h.mustRun("boards")
		if n := len(h.fake.Calls("InitiateAuth")); n != 2 {
			t.Errorf("refreshed token not reused: InitiateAuth calls = %d", n)
		}
	})

	t.Run("a refresh that fails means signed out", func(t *testing.T) {
		h := newCLIHarness(t)
		h.signIn()
		other, _ := rsa.GenerateKey(rand.Reader, 2048)
		setTokens(t, h, h.token(other), "revoked")
		code, _, errOut := h.run("", "boards")
		if code != 1 || !strings.Contains(errOut, "signed out") || !strings.Contains(errOut, "login") {
			t.Errorf("boards = %d %q", code, errOut)
		}
	})
}

func TestCLIReads(t *testing.T) {
	h := newCLIHarness(t)
	h.signIn()

	want := `board ` + cliBoard + ` "Roadmap"
  description "plans"
card ` + cliCard1 + ` "Backlog"
  description "later"
  task ` + cliTask1 + ` [pending] "Write tests"
    description "all of them"
  task ` + cliTask2 + ` [done] "Ship"
card ` + cliCard2 + ` "Doing"
  task ` + cliTask3 + ` [pending] "Review"
`
	if out := h.mustRun("board", cliBoard); out != want {
		t.Errorf("board =\n%s\nwant\n%s", out, want)
	}
	if out := h.mustRun("board", "b0000"); out != want {
		t.Errorf("board by prefix =\n%s", out)
	}

	var whole struct {
		Name  string
		Cards []struct{ ID string }
	}
	if err := json.Unmarshal([]byte(h.mustRun("board", cliBoard, "--json")), &whole); err != nil || whole.Name != "Roadmap" || len(whole.Cards) != 2 {
		t.Errorf("board --json = %+v, %v", whole, err)
	}
	var list struct{ Boards []struct{ ID string } }
	if err := json.Unmarshal([]byte(h.mustRun("--json", "boards")), &list); err != nil || len(list.Boards) != 1 {
		t.Errorf("boards --json = %+v, %v", list, err)
	}

	for _, c := range []struct{ args, want string }{
		{"board nope", `no board with id "nope"`},
	} {
		code, _, errOut := h.run("", strings.Fields(c.args)...)
		if code != 1 || !strings.Contains(errOut, c.want) {
			t.Errorf("%s = %d %q, want %q", c.args, code, errOut, c.want)
		}
	}
}

func TestCLIWrites(t *testing.T) {
	h := newCLIHarness(t)
	h.signIn()
	id := func(c map[string]any, k string) string { s, _ := c[k].(string); return s }

	t.Run("new-board", func(t *testing.T) {
		out := h.mustRun("new-board", "Fresh", "--description", "d")
		if !strings.HasPrefix(out, "board ") || !strings.HasSuffix(out, ` "Fresh"`+"\n") {
			t.Errorf("new-board = %q", out)
		}
		if n := len(h.fake.Calls("TransactWriteItems")); n != 1 {
			t.Errorf("TransactWriteItems = %d, want 1", n)
		}
	})

	t.Run("add-card and add-task send client-made ids", func(t *testing.T) {
		out := h.mustRun("add-card", "b0000", "Done")
		cardID := strings.Fields(out)[1]
		out = h.mustRun("add-task", cliBoard, "aaaa1", "Plan")
		taskID := strings.Fields(out)[1]
		if !strings.HasSuffix(out, `[pending] "Plan"`+"\n") {
			t.Errorf("add-task = %q", out)
		}
		got := h.takeChanges()
		if len(got) != 2 || got[0]["type"] != "createCard" || id(got[0], "cardId") != cardID || got[0]["name"] != "Done" ||
			got[1]["type"] != "createTask" || id(got[1], "taskId") != taskID || got[1]["cardId"] != cliCard1 {
			t.Errorf("changes = %v", got)
		}
	})

	t.Run("edits send only what they change", func(t *testing.T) {
		h.mustRun("edit-card", cliBoard, "aaaa1", "--name", "Icebox")
		h.mustRun("edit-task", cliBoard, "dddd1", "--status", "done")
		h.mustRun("edit-task", cliBoard, cliTask3, "--description", "")
		got := h.takeChanges()
		want := []map[string]any{
			{"type": "updateCard", "boardId": cliBoard, "cardId": cliCard1, "name": "Icebox"},
			{"type": "updateTask", "boardId": cliBoard, "cardId": cliCard1, "taskId": cliTask1, "status": "done"},
			{"type": "updateTask", "boardId": cliBoard, "cardId": cliCard2, "taskId": cliTask3, "description": ""},
		}
		if !jsonEqual(got, want) {
			t.Errorf("changes = %v\nwant %v", got, want)
		}
		if code, _, errOut := h.run("", "edit-task", cliBoard, cliTask1, "--status", "blocked"); code != 2 || !strings.Contains(errOut, "pending or done") {
			t.Errorf("bad status = %d %q", code, errOut)
		}
		if code, _, _ := h.run("", "edit-card", cliBoard, cliCard1); code != 2 {
			t.Errorf("edit-card without flags = %d, want 2", code)
		}
	})

	t.Run("moves name their neighbours", func(t *testing.T) {
		h.mustRun("move-card", cliBoard, cliCard1)
		h.mustRun("move-card", cliBoard, cliCard2, "--before", "aaaa1")
		h.mustRun("move-task", cliBoard, cliTask1, "--card", "aaaa2")
		h.mustRun("move-task", cliBoard, cliTask1, "--after", cliTask2)
		h.mustRun("move-task", cliBoard, cliTask2, "--before", cliTask1)
		got := h.takeChanges()
		want := []map[string]any{
			{"type": "moveCard", "boardId": cliBoard, "cardId": cliCard1, "beforeId": cliCard2, "afterId": nil},
			{"type": "moveCard", "boardId": cliBoard, "cardId": cliCard2, "beforeId": nil, "afterId": cliCard1},
			{"type": "moveTask", "boardId": cliBoard, "cardId": cliCard1, "taskId": cliTask1, "newCardId": cliCard2, "beforeId": cliTask3, "afterId": nil},
			{"type": "moveTask", "boardId": cliBoard, "cardId": cliCard1, "taskId": cliTask1, "newCardId": cliCard1, "beforeId": cliTask2, "afterId": nil},
			{"type": "moveTask", "boardId": cliBoard, "cardId": cliCard1, "taskId": cliTask2, "newCardId": cliCard1, "beforeId": nil, "afterId": cliTask1},
		}
		if !jsonEqual(got, want) {
			t.Errorf("changes = %v\nwant %v", got, want)
		}
		if code, _, errOut := h.run("", "move-task", cliBoard, cliTask1, "--before", cliTask1); code != 1 || !strings.Contains(errOut, "no task") {
			t.Errorf("task before itself = %d %q", code, errOut)
		}
		if code, _, errOut := h.run("", "move-card", cliBoard, cliCard1, "--before", cliCard2, "--after", cliCard2); code != 1 || !strings.Contains(errOut, "exclusive") {
			t.Errorf("before and after = %d %q", code, errOut)
		}
	})

	t.Run("deletes and clean-up", func(t *testing.T) {
		h.mustRun("delete-task", cliBoard, "dddd2")
		h.mustRun("delete-card", cliBoard, cliCard2)
		if out := h.mustRun("clean-up", cliBoard); out != "removed 1 done tasks\n" {
			t.Errorf("clean-up = %q", out)
		}
		got := h.takeChanges()
		want := []map[string]any{
			{"type": "deleteTask", "boardId": cliBoard, "cardId": cliCard1, "taskId": cliTask2},
			{"type": "deleteCard", "boardId": cliBoard, "cardId": cliCard2},
			{"type": "cleanUp", "boardId": cliBoard, "taskIds": []any{cliTask2}},
		}
		if !jsonEqual(got, want) {
			t.Errorf("changes = %v\nwant %v", got, want)
		}
	})

	t.Run("--json prints the change sent", func(t *testing.T) {
		out := h.mustRun("delete-card", cliBoard, cliCard1, "--json")
		if strings.TrimSpace(out) != `{"boardId":"`+cliBoard+`","cardId":"`+cliCard1+`","type":"deleteCard"}` {
			t.Errorf("--json = %q", out)
		}
	})

	t.Run("an ambiguous or unknown prefix is an error", func(t *testing.T) {
		for _, c := range []struct{ ref, want string }{
			{"aaaa", "ambiguous card"},
			{"zzzz", "no card"},
		} {
			code, _, errOut := h.run("", "delete-card", cliBoard, c.ref)
			if code != 1 || !strings.Contains(errOut, c.want) {
				t.Errorf("delete-card %s = %d %q, want %q", c.ref, code, errOut, c.want)
			}
		}
		if got := h.takeChanges(); len(got) != 1 { // only the --json delete above
			t.Errorf("failed lookups sent changes: %v", got)
		}
	})
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	var xa, ya any
	json.Unmarshal(x, &xa)
	json.Unmarshal(y, &ya)
	x, _ = json.Marshal(xa)
	y, _ = json.Marshal(ya)
	return string(x) == string(y)
}

const exported = `{
  "format": "skipli-board/1",
  "id": "` + cliBoard + `",
  "name": "Roadmap",
  "description": "plans",
  "cards": [
    {
      "id": "` + cliCard1 + `",
      "name": "Backlog",
      "description": "later",
      "tasks": [
        {
          "id": "` + cliTask1 + `",
          "name": "Write tests",
          "description": "all of them",
          "status": "pending"
        },
        {
          "id": "` + cliTask2 + `",
          "name": "Ship",
          "description": "",
          "status": "done"
        }
      ]
    },
    {
      "id": "` + cliCard2 + `",
      "name": "Doing",
      "description": "",
      "tasks": [
        {
          "id": "` + cliTask3 + `",
          "name": "Review",
          "description": "",
          "status": "pending"
        }
      ]
    }
  ]
}
`

func TestCLIExport(t *testing.T) {
	h := newCLIHarness(t)
	h.signIn()
	if out := h.mustRun("export", "b0000"); out != exported {
		t.Errorf("export =\n%s", out)
	}
	path := filepath.Join(h.dir, "roadmap.skipli.json")
	h.mustRun("export", cliBoard, path)
	if data, _ := os.ReadFile(path); string(data) != exported {
		t.Errorf("exported file =\n%s", data)
	}
	// What was exported shows the same as the board on the server.
	if h.mustRun("board", path) != h.mustRun("board", cliBoard) {
		t.Error("the file's board differs from the server's")
	}
}

func TestCLIImport(t *testing.T) {
	h := newCLIHarness(t)
	h.signIn()
	path := filepath.Join(h.dir, "roadmap.skipli.json")
	os.WriteFile(path, []byte(exported), 0o644)

	out := h.mustRun("import", path)
	f := strings.Fields(out)
	if len(f) != 3 || f[0] != "board" || f[1] == cliBoard || f[2] != `"Roadmap"` {
		t.Fatalf("import = %q, want a new board id", out)
	}
	if n := len(h.fake.Calls("TransactWriteItems")); n != 4 {
		t.Errorf("TransactWriteItems = %d, want 4 (the board, three task creates)", n)
	}

	id := f[1]
	got := h.takeChanges()
	for _, c := range got {
		delete(c, "createdAt")
	}
	want := []map[string]any{
		{"type": "createCard", "boardId": id, "cardId": cliCard1, "name": "Backlog"},
		{"type": "updateCard", "boardId": id, "cardId": cliCard1, "name": "Backlog", "description": "later"},
		{"type": "createTask", "boardId": id, "cardId": cliCard1, "taskId": cliTask1, "name": "Write tests"},
		{"type": "updateTask", "boardId": id, "cardId": cliCard1, "taskId": cliTask1, "name": "Write tests", "description": "all of them", "status": "pending"},
		{"type": "createTask", "boardId": id, "cardId": cliCard1, "taskId": cliTask2, "name": "Ship"},
		{"type": "updateTask", "boardId": id, "cardId": cliCard1, "taskId": cliTask2, "name": "Ship", "description": "", "status": "done"},
		{"type": "createCard", "boardId": id, "cardId": cliCard2, "name": "Doing"},
		{"type": "createTask", "boardId": id, "cardId": cliCard2, "taskId": cliTask3, "name": "Review"},
	}
	if !jsonEqual(got, want) {
		t.Errorf("changes =\n%v\nwant\n%v", got, want)
	}

	os.WriteFile(path, []byte(`{"format":"skipli-board/2"}`), 0o644)
	if code, _, errOut := h.run("", "import", path); code != 1 || !strings.Contains(errOut, "format") {
		t.Errorf("import of another format = %d %q", code, errOut)
	}
}
