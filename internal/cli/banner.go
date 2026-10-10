package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// lillaMolnet is printed when lillamolnet runs without arguments in a
// terminal. Never in --help, where people and agents look for a flag.
const lillaMolnet = `
          _/\/\/\_              <3
          |______|           <3    <3
      .-~~~~~~~~~~~~-.
   .-(                )-.
  (      o        o      )
 (                        )
  (       \______/       )
   '-.__________________.-'

    ~ lilla molnet cli ~
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
