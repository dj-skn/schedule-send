// Package worker is what a job's timer runs: it re-checks the target, sends
// the payload, and confirms an agent actually took the prompt.
package worker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dj-skn/schedule-send/internal/herdr"
	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/target"
)

const (
	// busyWaitMs is how long --if-busy wait gives a working agent to settle.
	busyWaitMs = 900000
	// limitWatch is how long the job stays open after a submit, which is long
	// enough for a rejected request to come back as a usage-limit notice.
	limitWatch = 10 * time.Second
)

// Herdr is the part of herdr the worker drives.
type Herdr interface {
	Targets() ([]target.Row, error)
	Read(pane, source string, lines int) (string, error)
	State(pane string) (status string, seq int64)
	Prompt(pane, msg string) (string, error)
	SendKeys(surface, pane string, keys []string, stderr io.Writer) error
	Run(pane, msg string, stderr io.Writer) error
	Wait(pane string, timeoutMs int) error
	Focus(pane string) error
	Focused() string
	Notify(title, body string)
}

// Clock is the worker's time, replaced in tests.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// Worker fires one job.
type Worker struct {
	ID    string
	Paths job.Paths
	Sys   job.Systemctl
	Out   io.Writer
	Err   io.Writer
	Clock Clock
	// Connect returns the herdr to drive for a binary path.
	Connect func(bin string) Herdr

	h       Herdr
	quiet   bool
	recover string
	grace   time.Duration

	sawDraft bool
	recovery string
}

// Fire runs the job with the given id and returns the exit status.
func Fire(id string) int {
	w := &Worker{
		ID: id, Paths: job.DefaultPaths(), Out: os.Stdout, Err: os.Stderr, Clock: realClock{},
		Connect: func(bin string) Herdr { return herdr.Client{Bin: bin, Stderr: os.Stderr} },
	}
	return w.Run()
}

// failed ends a run: the message has been reported and the exit status is 1.
var failed = errors.New("failed")

// Run fires the job and returns the exit status.
func (w *Worker) Run() int {
	statePath := w.Paths.StatePath(w.ID)
	if _, err := os.Stat(statePath); err != nil {
		fmt.Fprintf(w.Err, "No state file for %s\n", w.ID)
		return 1
	}
	st, err := job.Load(statePath)
	if err != nil {
		return w.refuse(st, "The state file for %s cannot be read: %v", w.ID, err)
	}
	if err := st.Validate(w.ID); err != nil {
		return w.refuse(st, "Invalid state file for %s: %v", w.ID, err)
	}
	if w.fire(st) != nil {
		return 1
	}
	if err := w.Paths.Cleanup(w.Sys, w.ID); err != nil {
		fmt.Fprintf(w.Err, "Job %s was sent but cleanup failed: %v\n", w.ID, err)
		return 1
	}
	return 0
}

// refuse reports a job whose state cannot be trusted and keeps its files for
// inspection. Its timer has elapsed and will not fire again, so a Herdr
// notification is the only notice the user gets. Herdr is used for nothing
// else: none of this state is delivered. A quiet flag that itself failed to
// load reads as not quiet.
func (w *Worker) refuse(st job.State, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(w.Err, msg)
	if st.Quiet == 1 {
		return 1
	}
	bin := st.HerdrBin
	if !executable(bin) {
		bin, _ = exec.LookPath("herdr")
	}
	if executable(bin) {
		w.Connect(bin).Notify("Scheduled send failed", fmt.Sprintf("%s. The job is kept; inspect it with schedule-send --status %s.", msg, w.ID))
	}
	return 1
}

// fail reports msg to the journal and as a Herdr notification, removes the
// job, and returns failed.
func (w *Worker) fail(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(w.Err, msg)
	if !w.quiet && w.h != nil {
		w.h.Notify("Scheduled send failed", msg)
	}
	if err := w.Paths.Cleanup(w.Sys, w.ID); err != nil {
		fmt.Fprintf(w.Err, "Job %s cleanup failed: %v\n", w.ID, err)
	}
	return failed
}

func (w *Worker) fire(st job.State) error {
	w.quiet = st.Quiet == 1
	w.recover = st.Recover
	w.grace = time.Duration(st.Grace) * time.Second

	bin := st.HerdrBin
	if !executable(bin) {
		bin, _ = exec.LookPath("herdr")
	}
	if !executable(bin) {
		return w.fail("herdr binary is unavailable at fire time.")
	}
	w.h = w.Connect(bin)

	pane := st.PaneID
	if st.Force != 1 {
		if err := w.check(st); err != nil {
			return err
		}
	}

	if st.Mode == "keys" {
		surface := "pane"
		if st.Kind == "agent" {
			surface = "agent"
		}
		if w.h.SendKeys(surface, pane, strings.Fields(st.Keys), w.Err) != nil {
			return w.fail("send-keys to %s failed.", pane)
		}
		fmt.Fprintf(w.Out, "sent keys to %s: %s\n", pane, st.Keys)
		return nil
	}

	b, err := os.ReadFile(w.Paths.MessagePath(w.ID))
	if err != nil {
		return w.fail("The message for %s is missing; send aborted.", w.ID)
	}
	msg := strings.TrimRight(string(b), "\n")

	if st.Kind != "agent" {
		if w.h.Run(pane, msg, w.Err) != nil {
			return w.fail("pane run in %s failed.", pane)
		}
		fmt.Fprintf(w.Out, "ran text in %s\n", pane)
		return nil
	}

	if st.IfBusy == "abort" {
		if status, _ := w.h.State(pane); status != "idle" && status != "done" {
			return w.fail("Agent in %s is %s and --if-busy=abort was set.", pane, status)
		}
	} else {
		// A wait that timed out is not a settled agent, and prompting it anyway
		// would type into a turn that is still running. Only "working" is
		// refused: an unknown status is Codex's flapping title, which the submit
		// gate is there to see through.
		_ = w.h.Wait(pane, busyWaitMs)
		if status, _ := w.h.State(pane); status == "working" {
			return w.fail("Agent in %s was still working after %d minutes; send aborted.", pane, busyWaitMs/60000)
		}
	}
	// The pane may have been replaced or moved during the long busy wait.
	if st.Force != 1 {
		if err := w.check(st); err != nil {
			return err
		}
	}
	return w.deliver(pane, msg)
}

// check re-resolves the target: it must still be what it was when scheduled.
func (w *Worker) check(st job.State) error {
	pane := st.PaneID
	rows, err := w.h.Targets()
	if err != nil {
		return w.fail("Could not list herdr's panes to check %s; send aborted.", pane)
	}
	var found *target.Row
	for i := range rows {
		if rows[i].Pane == pane {
			found = &rows[i]
			break
		}
	}
	switch {
	case found == nil:
		return w.fail("Target pane %s no longer exists; send aborted.", pane)
	case found.Kind != st.Kind:
		return w.fail("Pane %s is no longer a %s; send aborted.", pane, st.Kind)
	case st.Kind == "agent" && found.Agent != st.Agent:
		return w.fail("Pane %s now runs %s, not %s; send aborted.", pane, found.Agent, st.Agent)
	case found.Cwd != st.Cwd:
		return w.fail("Pane %s moved to %s; send aborted.", pane, found.Cwd)
	}
	return nil
}

func executable(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}
