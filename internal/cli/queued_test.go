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
	units := strings.Fields(os.Getenv("SYSTEMCTL_STANDIN_ACTIVE"))
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

// limitA makes wA:p1 show the same weekly limit as wB:p1.
func limitA(t *testing.T, h *harness) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, "herdr", "visible-wB-p1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(h.dir, "herdr", "visible-wA-p1.txt"), b, 0o644)
}

// twoLimited is the owner's screenshot: wA:p1 and wB:p1 held by one limit,
// and wA:p1 already has a send for 16:01, the time a run proposes.
func twoLimited(t *testing.T, h *harness) {
	t.Helper()
	limitA(t, h)
	h.schedule(t, job.State{ID: "20261003-160100-wAp1", Kind: "agent", PaneID: "wA:p1", Agent: "claude",
		Cwd: "/home/dev/projects/analytics", At: "2026-10-03 16:01:00", RunAt: 1791036060})
	h.armed(t, "20261003-160100-wAp1")
}

func TestPickerPutsTheScheduledAgentLast(t *testing.T) {
	input := "\n\n\n"
	h := newHarness(t, &input)
	twoLimited(t, h)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
◆  Which agent or pane?
│  agents
│    1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
│    2  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
│    3  ● working  cursor   wE:p1    ~/src/schedule-send        Tidy schedule-send UI
│    4  » queued   claude   wA:p1    ~/projects/analytics       Analytics backend rewrite  resets 16:00 (weekly)  sends 16:01
│
│  panes
│    5  · zsh      wB:p5    ~/projects/shop-backend
│
│  Enter takes 1, or type its number
`
	if !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant within it:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wB:p1 (claude, idle)\n") {
		t.Errorf("Enter did not take the agent still waiting:\n%s", h.stdout)
	}
}

func TestPickBestSkipsTheScheduledAgent(t *testing.T) {
	h := newHarness(t, nil)
	twoLimited(t, h)
	if code := h.a.main([]string{"--pick", "best", "--auto", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.HasPrefix(h.stderr.String(), "Multiple targets matched; picking claude in wB:p1 ") {
		t.Errorf("stderr:\n%s", h.stderr)
	}
}

func TestPickerMarksAScheduledPaneAndCountsSends(t *testing.T) {
	h := newHarness(t, nil)
	for _, at := range []struct {
		id    string
		runAt int64
	}{{"20261003-170000-wBp5", 1791039600}, {"20261003-163000-wBp5", 1791037800}} {
		h.schedule(t, job.State{ID: at.id, Kind: "pane", PaneID: "wB:p5", Agent: "-",
			Cwd: "/home/dev/projects/shop-backend", At: "2026-10-03 16:30:00", RunAt: at.runAt, Mode: "keys", Keys: "enter", Grace: 4})
	}
	h.armed(t, "20261003-170000-wBp5", "20261003-163000-wBp5")
	if code := h.a.main([]string{"--at", "16:30", "--message", "hi"}); code != 1 {
		t.Errorf("exit %d", code)
	}
	if want := "  5  » zsh      wB:p5    ~/projects/shop-backend  2 sends, next 16:30\n"; !strings.Contains(h.stderr.String(), want) {
		t.Errorf("stderr lacks %q:\n%s", want, h.stderr)
	}
}

// laterSend schedules a send to wA:p1 at 16:30, which is not the time a run
// proposes for it, so a new send can be armed beside it.
func laterSend(t *testing.T, h *harness) {
	t.Helper()
	h.schedule(t, job.State{ID: "20261003-163000-wAp1", Kind: "agent", PaneID: "wA:p1", Agent: "claude",
		Cwd: "/home/dev/projects/analytics", At: "2026-10-03 16:30:00", RunAt: 1791037800})
	h.armed(t, "20261003-163000-wAp1")
}

func TestThePluginsPromptSaysTheAgentHasASend(t *testing.T) {
	// The plugin passes the pane it opened on, so there is no picker; the
	// report that opens the questions says it instead.
	input := "\n\n"
	h := newHarness(t, &input)
	limitA(t, h)
	laterSend(t, h)
	if code := h.a.main([]string{"--pane", "wA:p1", "--message", "go on", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `│  claude · wA:p1 · ~/projects/analytics
│  ◷ hit its weekly limit · resets Sat 16:00, in 38m
│  » already has a send at Sat 16:30 · this adds another
│
◆  When?
`
	if !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant within it:\n%s", h.tty, want)
	}
	if strings.Contains(h.stderr.String(), "already has") {
		t.Errorf("said twice, on stderr too:\n%s", h.stderr)
	}
}

func TestAGivenTimeNotesTheSendsAlreadyThere(t *testing.T) {
	h := newHarness(t, nil)
	laterSend(t, h)
	if code := h.a.main([]string{"--pane", "wA:p1", "--at", "17:00", "--message", "x", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "wA:p1 already has a send at Sat 16:30; this adds another.\n"; !strings.Contains(h.stderr.String(), want) {
		t.Errorf("stderr lacks %q:\n%s", want, h.stderr)
	}

	t.Run("as JSON", func(t *testing.T) {
		h := newHarness(t, nil)
		laterSend(t, h)
		if code := h.a.main([]string{"--pane", "wA:p1", "--at", "17:00", "--message", "x", "--dry-run", "--json"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if want := `{"note":"wA:p1 already has a send at Sat 16:30; this adds another."}`; !strings.Contains(h.stderr.String(), want) {
			t.Errorf("stderr lacks %s:\n%s", want, h.stderr)
		}
	})

	t.Run("beside the rail after the picker", func(t *testing.T) {
		input := "4\n"
		h := newHarness(t, &input)
		laterSend(t, h)
		if code := h.a.main([]string{"--at", "17:00", "--message", "x", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if want := "│  › │\n│  » already has a send at Sat 16:30 · this adds another\n"; !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal lacks %q:\n%s", want, h.tty)
		}
		if !strings.Contains(h.stdout.String(), "target=agent wA:p1 ") {
			t.Errorf("row 4 is not wA:p1:\n%s", h.stdout)
		}
	})

	t.Run("nothing when there is none", func(t *testing.T) {
		h := newHarness(t, nil)
		if code := h.a.main([]string{"--pane", "wA:p1", "--at", "17:00", "--message", "x", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if strings.Contains(h.stderr.String(), "already has") {
			t.Errorf("stderr:\n%s", h.stderr)
		}
	})
}

func TestAClashAtWhenAsksAgainAndKeepsTheMessage(t *testing.T) {
	// wA:p1 already has a send at 16:01, the time the run proposes.
	for _, c := range []struct {
		name, input, said string
	}{
		{"Enter on the proposed time", "go on\n\n16:05\n\n", "│  › │  ✗ wA:p1 already has a send at Sat 16:01. Type another time.\n│  › │  → Sat 16:05 · in 43m"},
		{"the same time typed", "go on\n16:01\n16:05\n\n", "│  › │  ✗ wA:p1 already has a send at Sat 16:01. Type another time.\n│  › │  → Sat 16:05 · in 43m"},
	} {
		t.Run(c.name, func(t *testing.T) {
			input := c.input
			h := newHarness(t, &input)
			twoLimited(t, h)
			if code := h.a.main([]string{"--pane", "wA:p1", "--dry-run"}); code != 0 {
				t.Fatalf("exit %d: %s\n%s", code, h.stderr, h.tty)
			}
			if !strings.Contains(h.tty.String(), c.said) {
				t.Errorf("terminal:\n%s\nwant within it:\n%s", h.tty, c.said)
			}
			if want := `└  · Dry run: would schedule "go on" for Sat 16:05`; !strings.Contains(h.tty.String(), want) {
				t.Errorf("terminal lacks %q:\n%s", want, h.tty)
			}
		})
	}

	t.Run("--at is refused as before", func(t *testing.T) {
		h := newHarness(t, nil)
		twoLimited(t, h)
		if code := h.a.main([]string{"--pane", "wA:p1", "--at", "16:01", "--message", "x", "--dry-run"}); code != 1 {
			t.Errorf("exit %d", code)
		}
		if want := "A send to wA:p1 at 2026-10-03 16:01:00 is already scheduled (20261003-160100-wAp1)."; !strings.Contains(h.stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, h.stderr)
		}
	})
}

func TestThePickerFitsANarrowTerminal(t *testing.T) {
	input := "\n\n\n"
	h := newHarness(t, &input)
	h.a.ttyWidth = func() int { return 52 }
	twoLimited(t, h)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	for _, want := range []string{
		"┌  Schedule a send · dry run  Ctrl-C cancels\n",
		"│    1  ◷ limited  claude   wB:p1    …/shop-backend\n│       resets 16:00 (weekly)\n",
		"│    4  » queued   claude   wA:p1    …/analytics\n│       resets 16:00 (weekly)  sends 16:01\n",
	} {
		if !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal lacks %q:\n%s", want, h.tty)
		}
	}
	picker := h.tty.String()[:strings.Index(h.tty.String(), "Enter takes 1")]
	for _, line := range strings.Split(picker, "\n") {
		if n := len([]rune(line)); n > 52 {
			t.Errorf("%d columns wide: %q", n, line)
		}
	}
}
