package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// lillaMolnet is printed before the help for lillamolnet itself, but not
// before the help for a single command.
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

func printBanner(w io.Writer) {
	fmt.Fprint(w, lillaMolnet, "\n")
}

// withBanner prints the banner before the root command's help.
func withBanner(root *cobra.Command) {
	help := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root {
			printBanner(cmd.OutOrStdout())
		}
		help(cmd, args)
	})
}
