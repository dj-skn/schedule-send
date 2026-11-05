package cli

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/show"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/when"
)

type queuedSend struct {
	id string
	at int64
}

// queued returns the sends already scheduled to each pane, soonest first. A send counts when its state reads, it is for what
// the pane shows now (the test the worker makes before delivering), and its
// timer is armed, which a send being delivered still is. The state files are
// read on the first call only, and the scheduler is asked only about sends
// that match a pane.
func (a *app) queued() map[string][]queuedSend {
	if a.sends != nil {
		return a.sends
	}
	a.sends = map[string][]queuedSend{}
	panes := map[string]target.Row{}
	for _, r := range a.rows {
		panes[r.Pane] = r
	}
	for _, f := range a.paths.StateFiles() {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		s, err := job.Load(f)
		if err != nil || s.ID != id {
			continue
		}
		r, ok := panes[s.PaneID]
		if !ok || !sameTarget(r, s) {
			continue
		}
		at := runAt(s)
		if at == 0 || a.sys.Armed(id) != "active" {
			continue
		}
		a.sends[s.PaneID] = append(a.sends[s.PaneID], queuedSend{id: id, at: at})
	}
	for _, sends := range a.sends {
		slices.SortStableFunc(sends, func(x, y queuedSend) int { return cmp.Compare(x.at, y.at) })
	}
	return a.sends
}

// sameTarget reports whether a send's state is for what row shows now: the
// same kind, agent, and folder, as the worker checks before it delivers. A
// pane id alone can belong to another agent by then.
func sameTarget(r target.Row, s job.State) bool {
	return r.Kind == s.Kind && r.Cwd == s.Cwd && (s.Kind != "agent" || r.Agent == s.Agent)
}

// runAt is when a stored send fires: its explicit instant, or for an older
// state its calendar time read in the local zone; 0 when neither reads.
func runAt(s job.State) int64 {
	if s.RunAt != 0 {
		return s.RunAt
	}
	if at, err := time.ParseInLocation(when.Calendar, s.At, when.Loc); err == nil {
		return at.Unix()
	}
	return 0
}

// queuedLast moves the targets with a send already scheduled behind those
// without, agents and plain panes each in their own group, keeping the order
// otherwise. Enter, and --pick best, then take a target still waiting.
func queuedLast(rows []target.Row, sends map[string][]queuedSend) []target.Row {
	key := func(r target.Row) int {
		k := 0
		if !r.IsAgent() {
			k = 2
		}
		if len(sends[r.Pane]) > 0 {
			k++
		}
		return k
	}
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(x, y target.Row) int { return key(x) - key(y) })
	return out
}

// queuedMarks marks each target with a send scheduled, its note after any
// limit's, e.g. "resets 16:00  sends 16:01".
func queuedMarks(marks map[string]target.Mark, sends map[string][]queuedSend, now int64) {
	for pane, jobs := range sends {
		m := marks[pane]
		m.Queued = true
		if m.Note != "" {
			m.Note += "  "
		}
		m.Note += queuedNote(jobs, now)
		marks[pane] = m
	}
}

// queuedNote is how the picker says when a target's sends fire: "sends
// 16:01", or "2 sends, next 16:01".
func queuedNote(sends []queuedSend, now int64) string {
	if len(sends) == 1 {
		return "sends " + noteTime(sends[0].at, now)
	}
	return fmt.Sprintf("%d sends, next %s", len(sends), noteTime(sends[0].at, now))
}

func (a *app) queuedTimes(pane string) []int64 {
	sends := a.queued()[pane]
	times := make([]int64, 0, len(sends))
	for _, send := range sends {
		times = append(times, send.at)
	}
	return times
}

// tellQueued says that the target already has sends, before another is
// armed: beside the rail when the picker opened one, else as a note.
func (a *app) tellQueued(t target.Row) {
	times := a.queuedTimes(t.Pane)
	if len(times) == 0 {
		return
	}
	if a.ask != nil {
		a.ask.say(a.ask.rail.Line(show.QueuedLine(a.ask.c, times, a.now)))
		return
	}
	a.note(fmt.Sprintf("%s already has %s; this adds another.", t.Pane, show.QueuedPhrase(times, a.now)))
}

// sendID names a send by when it fires and where, so two sends to one pane
// in the same second would share their state and units.
func sendID(runAt int64, pane string) string {
	return when.Format(runAt, when.Stamp) + "-" + alnum(pane)
}

// taken reports whether the target already has a send stored for runAt,
// which arming would refuse. The question asks again instead, so what was
// typed before it is kept.
func (a *app) taken(runAt int64) bool {
	_, err := os.Stat(a.paths.StatePath(sendID(runAt, a.tgt.Pane)))
	return err == nil
}

func (a *app) takenMsg(runAt, now int64) string {
	return fmt.Sprintf("%s already has a send at %s. Type another time.", a.tgt.Pane, show.Time(runAt, now))
}
