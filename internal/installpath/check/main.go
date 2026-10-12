// Command check lets tools/build refuse an installation path with the same
// rules arming applies, before anything is built.
package main

import (
	"fmt"
	"os"

	"github.com/dj-skn/schedule-send/internal/installpath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: check INSTALLED_PATH")
		os.Exit(2)
	}
	if err := installpath.Check(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid installed path %q: %v.\n", os.Args[1], err)
		os.Exit(1)
	}
}
