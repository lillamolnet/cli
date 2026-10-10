// Command lillamolnet is the command line interface for Lilla Molnet.
package main

import (
	"os"

	"github.com/lillamolnet/cli/internal/cli"
)

func main() {
	in := cli.Input{Reader: os.Stdin, Terminal: isTerminal(os.Stdin)}
	os.Exit(cli.Execute(os.Args[1:], in, os.Stdout, os.Stderr, os.Getenv))
}

// isTerminal reports whether f is a terminal, so that the CLI never asks
// questions when stdin is a pipe or a file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
