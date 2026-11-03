package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/target"
)

// schedule stores a send as arming would, without arming anything; armed
// makes the stand-in scheduler answer that its timer is armed, on either
// system.
func (h *harness) schedule(t *testing.T, s job.State) {
	t.Helper()
	if s.Mode == "" {
		s.Mode, s.Grace = "text", 4
	}
	msg := "resume"
	if err := h.a.paths.Write(s, &msg); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) armed(t *testing.T, ids ...string) {
	t.Helper()
	var units []string
	dir := filepath.Join(h.dir, "launchd")
	os.MkdirAll(dir, 0o755)
	for _, id := range ids {
		units = append(units, job.UnitPrefix+"-"+id+".timer")
		os.WriteFile(filepath.Join(dir, job.UnitPrefix+"-"+id), nil, 0o644)
	}
	t.Setenv("SYSTEMCTL_STANDIN_ACTIVE", strings.Join(units, " "))
	t.Setenv("LAUNCHCTL_STANDIN_DIR", dir)
}

func TestQueuedCountsArmedSendsForWhatThePaneShows(t *testing.T) {
	agent := func(pane, cwd string) target.Row {
		return target.Row{Kind: "agent", Agent: "claude", Pane: pane, Cwd: cwd, Status: "idle"}
	}
	rows := []target.Row{
		agent("wA:p1", "/home/dev/projects/analytics"),
		agent("wB:p1", "/home/dev/projects/shop-backend"),
		{Kind: "pane", Agent: "-", Pane: "wB:p5", Cwd: "/home/dev/projects/shop-backend"},
	}
	send := func(id, pane, kind, agentName, cwd string, runAt int64) job.State {
		return job.State{ID: id, Kind: kind, PaneID: pane, Agent: agentName, Cwd: cwd, At: "2026-10-03 16:00:00", RunAt: runAt}
	}
	h := newHarness(t, nil)
	h.a.rows = rows
	h.schedule(t, send("20261003-170000-wAp1", "wA:p1", "agent", "claude", "/home/dev/projects/analytics", 1791040800))
	h.schedule(t, send("20261003-160000-wAp1", "wA:p1", "agent", "claude", "/home/dev/projects/analytics", 1791037200))
	// The same pane id, but another agent or folder: the worker would abort it.
	h.schedule(t, send("20261003-160000-wBp1", "wB:p1", "agent", "codex", "/home/dev/projects/shop-backend", 1791037200))
	h.schedule(t, send("20261003-161000-wBp1", "wB:p1", "agent", "claude", "/home/dev/elsewhere", 1791037800))
	// A plain pane counts too.
	h.schedule(t, send("20261003-162000-wBp5", "wB:p5", "pane", "-", "/home/dev/projects/shop-backend", 1791038400))
	// Stored but not armed: its timer is gone.
	h.schedule(t, send("20261003-163000-wBp5", "wB:p5", "pane", "-", "/home/dev/projects/shop-backend", 1791039000))
	// A state that does not read is never counted.
	os.WriteFile(filepath.Join(h.a.paths.StateDir, "20261003-164000-wAp1.json"), []byte("{}"), 0o600)
	h.armed(t, "20261003-170000-wAp1", "20261003-160000-wAp1", "20261003-160000-wBp1",
		"20261003-161000-wBp1", "20261003-162000-wBp5", "20261003-164000-wAp1")

	got := h.a.queued()
	want := map[string][]int64{
		"wA:p1": {1791037200, 1791040800},
		"wB:p5": {1791038400},
	}
	if len(got) != len(want) {
		t.Fatalf("queued = %v, want %v", got, want)
	}
	for pane, times := range want {
		if !slices.Equal(got[pane], times) {
			t.Errorf("queued[%s] = %v, want %v", pane, got[pane], times)
		}
	}

	t.Run("read once", func(t *testing.T) {
		os.RemoveAll(h.a.paths.StateDir)
		if len(h.a.queued()["wA:p1"]) != 2 {
			t.Errorf("queued was read again: %v", h.a.queued())
		}
	})
}

func TestQueuedWithoutStateIsEmpty(t *testing.T) {
	h := newHarness(t, nil)
	h.a.rows = []target.Row{{Kind: "agent", Agent: "claude", Pane: "wA:p1"}}
	if got := h.a.queued(); len(got) != 0 {
		t.Errorf("queued = %v", got)
	}
}
