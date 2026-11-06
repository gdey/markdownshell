// Command stressdoc writes a long markdown document of one shape, for
// trying mdsh-gui on it:
//
//	go run ./internal/stress/stressdoc -shape runbook -lines 100000 > big.md
//
// The shapes are those of package stress.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/internal/stress"
)

func main() {
	shape := flag.String("shape", "runbook", "the document's shape: "+strings.Join(stress.Shapes, ", "))
	lines := flag.Int("lines", 10000, "about how many lines to write")
	flag.Parse()
	if !strings.Contains(" "+strings.Join(stress.Shapes, " ")+" ", " "+*shape+" ") {
		fmt.Fprintf(os.Stderr, "stressdoc: no shape %q; the shapes are %s\n", *shape, strings.Join(stress.Shapes, ", "))
		os.Exit(2)
	}
	os.Stdout.Write(stress.Write(*shape, *lines))
}
