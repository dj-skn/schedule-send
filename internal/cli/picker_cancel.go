package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
)

// cancelQueuedSend returns false with no error when the user goes back.
func (a *app) cancelQueuedSend(q *asker, row target.Row, sends []queuedSend) (bool, error) {
	send := sends[0]
	if len(sends) > 1 {
		n, err := chooseQueuedSend(q, sends)
		if err != nil || n == 0 {
			return false, err
		}
		send = sends[n-1]
	}

	// Keep the selected state file open so its inode cannot be reused while
	// the user answers. The removal lock is taken only after confirmation.
	stateFile, err := os.Open(a.paths.StatePath(send.id))
	if errors.Is(err, os.ErrNotExist) {
		return false, queuedSendChanged(send.id)
	}
	if err != nil {
		return false, die("Could not open queued send %s: %v", term.Clean(send.id), err)
	}
	defer stateFile.Close()
	state, err := job.ReadState(stateFile)
	if err != nil || state.ID != send.id || state.PaneID != row.Pane ||
		!sameTarget(row, state) || runAt(state) != send.at {
		return false, queuedSendChanged(send.id)
	}
	selected, err := stateFile.Stat()
	if err != nil {
		return false, die("Could not inspect queued send %s: %v", term.Clean(send.id), err)
	}

	q.say(q.rail.Gap(), q.rail.Step("Cancel this send?"),
		q.rail.Line(fmt.Sprintf("%s at %s", term.Clean(row.Pane), when.Format(send.at, when.Calendar))),
		q.rail.Line("ID "+term.Clean(send.id)),
		q.rail.Hint("Type y to cancel, Enter or n to go back"))
	for {
		answer, ok, err := q.answer()
		if err != nil {
			return false, err
		}
		if !ok {
			return false, die("Cancellation confirmation ended without an answer.")
		}
		switch answer {
		case "y":
			a.doneAsking()
			return true, a.cancel(send.id, selected)
		case "", "n":
			return false, nil
		default:
			q.say(q.rail.Error("Type y or n, or press Enter."))
		}
	}
}

func chooseQueuedSend(q *asker, sends []queuedSend) (int, error) {
	q.say(q.rail.Gap(), q.rail.Step("Which queued send?"))
	for i, send := range sends {
		q.say(q.rail.Line(fmt.Sprintf("%d  %s", i+1, when.Format(send.at, when.Calendar))),
			q.rail.Line("   ID "+term.Clean(send.id)))
	}
	q.say(q.rail.Hint("Type its number, or Enter to go back"))
	for {
		answer, ok, err := q.answer()
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, die("Queued send selection ended without an answer.")
		}
		if answer == "" {
			return 0, nil
		}
		if n := pickerNumber(answer, len(sends)); n != 0 {
			return n, nil
		}
		q.say(q.rail.Error(fmt.Sprintf("Enter a number from 1 to %d.", len(sends))))
	}
}

func queuedSendChanged(id string) error {
	return die("Queued send %s changed or is gone. Open the picker again.", term.Clean(id))
}
