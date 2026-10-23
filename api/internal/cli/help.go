package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

const overview = `skipli-cli reads and changes Skipli boards as the account signed in to it.

Usage: skipli-cli [--env dev] <command> [arguments] [flags]

Commands:
`

const notes = `
Ids
  Boards, cards and tasks are named by id only, never by name. Any unique
  prefix of an id works (it is checked against the board; an unknown or
  ambiguous prefix is an error). Run "boards", then "board <id>", to see ids.

Output
  Compact text, one item per line, with full ids. Add --json to any command
  for JSON: the API's own for reads, the Board change sent for writes.
  Errors go to stderr as "skipli-cli: <message>" with a non-zero exit status
  (2 for a usage error). Nothing is retried, except one sign-in refresh.

Environment
  Production by default; --env dev for Development. Each has its own stored
  Session in ~/.config/skipli/<env>.json. For a local API, pass --url <base>
  or set SKIPLI_URL.

Signing in
  Signing in is the account owner's job. Without a terminal, "login <email>"
  mails a code and exits; "login --code <code>" finishes it. A Session lasts
  30 days; when it is over, commands say to run login again.

Task statuses are "pending" and "done". Moves go to the end unless --before
or --after names a neighbour.
`

func printHelp(w io.Writer, topic string) error {
	if topic == "" {
		fmt.Fprint(w, overview)
		for i := range commands {
			fmt.Fprintf(w, "  %s\n", usageLine(&commands[i]))
		}
		fmt.Fprint(w, "  help [command]\n")
		fmt.Fprint(w, notes)
		fmt.Fprintln(w, "\nRun \"skipli-cli help <command>\" for a command's flags.")
		return nil
	}
	cmd := findCommand(topic)
	if cmd == nil {
		return usageError{fmt.Sprintf("no help for %q", topic)}
	}
	fmt.Fprintf(w, "Usage: skipli-cli %s\n\n%s\n", usageLine(cmd), cmd.help)
	fs := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	fmt.Fprintln(w, "\nFlags:")
	fs.VisitAll(func(f *flag.Flag) { fmt.Fprintf(w, "  --%s  %s\n", f.Name, f.Usage) })
	fmt.Fprintln(w, "  --json  print the API's JSON")
	fmt.Fprintln(w, "  --env prod|dev  the Environment (default prod)")
	fmt.Fprintln(w, "  --url <base>  base URL of a local API (or SKIPLI_URL)")
	return nil
}

// usageLine is a command's name and arguments, such as
// "move-task <board> <task> [--card ...]".
func usageLine(c *command) string {
	parts := []string{c.name}
	for i, a := range c.args {
		if i < c.min {
			parts = append(parts, "<"+a+">")
		} else {
			parts = append(parts, "["+a+"]")
		}
	}
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	if c.flags != nil {
		c.flags(fs)
	}
	fs.VisitAll(func(f *flag.Flag) { parts = append(parts, "[--"+f.Name+"]") })
	return strings.Join(parts, " ")
}
