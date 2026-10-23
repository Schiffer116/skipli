// Package cli is skipli-cli: it reads and changes Skipli boards through the
// HTTP API, as the account signed in to it. It sends the same Board changes
// the app does and keeps no Offline copy.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Options is everything Run needs from the process around it.
type Options struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Interactive is whether Stdin is a terminal a person can type a
	// sign-in code into.
	Interactive bool
	Getenv      func(string) string
	// ConfigDir holds one stored Session per Environment.
	ConfigDir  string
	HTTPClient *http.Client
}

// env is one command's run: where it talks to, as whom, and where its output goes.
type env struct {
	Options
	client *client
	json   bool
	envArg string // --env, for the follow-up commands we print
	urlArg string // --url
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// command is one subcommand; commands.go lists them.
type command struct {
	name  string
	args  []string // positional arguments, in order
	min   int      // how many of args are required
	help  string
	flags func(fs *flag.FlagSet)
	run   func(ctx context.Context, e *env, a *args) error
}

type args struct {
	pos []string
	fs  *flag.FlagSet
}

func (a *args) str(name string) string { return a.fs.Lookup(name).Value.String() }

func (a *args) set(name string) bool {
	set := false
	a.fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// Run runs one skipli-cli command and returns its exit status.
func Run(ctx context.Context, o Options) int {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Getenv == nil {
		o.Getenv = func(string) string { return "" }
	}
	err := run(ctx, o)
	var usage usageError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.As(err, &usage):
		fmt.Fprintf(o.Stderr, "skipli-cli: %s\nrun `skipli-cli help` for usage\n", usage.msg)
		return 2
	}
	fmt.Fprintf(o.Stderr, "skipli-cli: %s\n", err)
	return 1
}

func run(ctx context.Context, o Options) error {
	// The command is the first word that isn't a flag or a flag's value;
	// global flags may come before it.
	name, at := "", -1
	for i := 0; i < len(o.Args); i++ {
		arg := o.Args[i]
		if arg == "-h" || arg == "--help" || !strings.HasPrefix(arg, "-") {
			name, at = arg, i
			break
		}
		if arg == "--env" || arg == "--url" || arg == "-env" || arg == "-url" {
			i++
		}
	}
	var rest []string
	if at >= 0 {
		rest = slices.Concat(o.Args[:at], o.Args[at+1:])
	}
	if name == "" || name == "-h" || name == "--help" {
		return printHelp(o.Stdout, "")
	}
	if name == "help" {
		topic := ""
		if len(rest) > 0 {
			topic = rest[0]
		}
		return printHelp(o.Stdout, topic)
	}
	cmd := findCommand(name)
	if cmd == nil {
		return usageError{fmt.Sprintf("unknown command %q", name)}
	}

	fs := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	envName := fs.String("env", "", "Environment: prod (default) or dev")
	baseURL := fs.String("url", "", "base URL of the API, for a local one (or set SKIPLI_URL)")
	asJSON := fs.Bool("json", false, "print the API's JSON")
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	pos, err := parseInterspersed(fs, rest)
	if errors.Is(err, flag.ErrHelp) {
		return printHelp(o.Stdout, cmd.name)
	}
	if err != nil {
		return usageError{fmt.Sprintf("%s: %v", cmd.name, err)}
	}
	if len(pos) < cmd.min || len(pos) > len(cmd.args) {
		return usageError{fmt.Sprintf("usage: %s", usageLine(cmd))}
	}

	base, envKey, err := baseURLFor(*envName, firstNonEmpty(*baseURL, o.Getenv("SKIPLI_URL")))
	if err != nil {
		return usageError{err.Error()}
	}
	e := &env{Options: o, json: *asJSON, envArg: *envName, urlArg: *baseURL}
	path := sessionFile(o.ConfigDir, envKey, base)
	sess, err := loadSession(path)
	if err != nil {
		return err
	}
	e.client = &client{base: base, http: o.HTTPClient, path: path, sess: sess}
	return cmd.run(ctx, e, &args{pos: pos, fs: fs})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// baseURLFor picks the API: a URL override, else the Environment (Production
// by default). envKey is "" for an override.
func baseURLFor(envName, override string) (base, envKey string, err error) {
	if override != "" {
		return strings.TrimRight(override, "/"), "", nil
	}
	if envName == "" {
		envName = "prod"
	}
	base, ok := environments[envName]
	if !ok {
		return "", "", fmt.Errorf("unknown Environment %q, want prod or dev", envName)
	}
	return base, envName, nil
}

// parseInterspersed parses flags that may come before, between or after the
// positional arguments; "--" ends the flags.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var literal []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, literal = args[:i], args[i+1:]
	}
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	return append(pos, literal...), nil
}

// follow is this invocation's own prefix for commands we tell the user to run.
func (e *env) follow() string {
	cmd := "skipli-cli"
	if e.envArg != "" {
		cmd += " --env " + e.envArg
	}
	if e.urlArg != "" {
		cmd += " --url " + e.urlArg
	}
	return cmd
}
