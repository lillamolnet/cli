// Command lillamolnet is the command line interface for Lilla Molnet.
package main

import (
	"os"

	"github.com/lillamolnet/cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
