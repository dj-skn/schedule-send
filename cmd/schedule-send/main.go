// Command schedule-send schedules text or key presses into a Herdr agent or
// pane, through an on-disk systemd --user timer. See README.md.
package main

import (
	"os"

	"schedule-send/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
