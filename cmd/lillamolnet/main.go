// Command lillamolnet is the command line interface for Lilla Molnet.
package main

import (
	"os"

	"github.com/lillamolnet/cli/internal/cli"
	"golang.org/x/term"
)

func main() {
	// A real terminal, so that the CLI never asks questions when stdin is a
	// pipe, a file or /dev/null. /dev/null is a character device too, so the
	// file mode is not enough.
	in := cli.Input{Reader: os.Stdin, Terminal: term.IsTerminal(int(os.Stdin.Fd()))}
	os.Exit(cli.Execute(os.Args[1:], in, os.Stdout, os.Stderr, os.Getenv))
}
