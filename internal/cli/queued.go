package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/when"
)

// queued is when the sends already scheduled to each pane fire, soonest
// first, keyed by pane. A send counts when its state reads, it is for what
// the pane shows now (the test the worker makes before delivering), and its
// timer is armed, which a send being delivered still is. The state files are
// read on the first call only, and the scheduler is asked only about sends
// that match a pane.
func (a *app) queued() map[string][]int64 {
	if a.sends != nil {
		return a.sends
	}
	a.sends = map[string][]int64{}
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
		a.sends[s.PaneID] = append(a.sends[s.PaneID], at)
	}
	for _, times := range a.sends {
		slices.Sort(times)
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
func queuedLast(rows []target.Row, sends map[string][]int64) []target.Row {
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
func queuedMarks(marks map[string]target.Mark, sends map[string][]int64, now int64) {
	for pane, times := range sends {
		m := marks[pane]
		m.Queued = true
		if m.Note != "" {
			m.Note += "  "
		}
		m.Note += queuedNote(times, now)
		marks[pane] = m
	}
}

// queuedNote is how the picker says when a target's sends fire: "sends
// 16:01", or "2 sends, next 16:01".
func queuedNote(times []int64, now int64) string {
	if len(times) == 1 {
		return "sends " + noteTime(times[0], now)
	}
	return fmt.Sprintf("%d sends, next %s", len(times), noteTime(times[0], now))
}
