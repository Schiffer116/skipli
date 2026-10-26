// Command skipli-cli reads and changes Skipli boards from a terminal, as the
// account signed in to it. The work is in internal/cli.
package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/Schiffer116/skipli/api/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, cli.Options{
		Args:        os.Args[1:],
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: isTerminal(os.Stdin),
		Getenv:      os.Getenv,
		ConfigDir:   configDir(),
	}))
}

func configDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "skipli")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "skipli")
}

// isTerminal reports whether f is a terminal, not a pipe, a file or
// /dev/null, which is what an agent's shell usually gives a command.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(info, null)
}
