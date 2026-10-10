package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// lillaMolnet is printed before the help for lillamolnet itself, but not
// before the help for a single command, and only in a terminal.
const lillaMolnet = `
          _/\/\/\_              <3
          |______|           <3    <3
      .-~~~~~~~~~~~~-.
   .-(                )-.
  (      o        o      )
 (                        )
  (       \______/       )
   '-.__________________.-'

        ~ lilla molnet ~
`

// isTerminal reports whether w is a terminal. A variable, so that tests can
// pretend.
var isTerminal = defaultIsTerminal

func defaultIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func printBanner(w io.Writer) {
	fmt.Fprint(w, lillaMolnet, "\n")
}

// withBanner prints the banner before the root command's help, when it goes
// to a terminal. Scripts and agents that read the help get it without.
func withBanner(root *cobra.Command) {
	help := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root && isTerminal(cmd.OutOrStdout()) {
			printBanner(cmd.OutOrStdout())
		}
		help(cmd, args)
	})
}
