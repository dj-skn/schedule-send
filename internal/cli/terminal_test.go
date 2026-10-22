package cli

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dj-skn/schedule-send/internal/limit"
	"github.com/dj-skn/schedule-send/internal/show"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
)

func TestUnknownArgumentExitsTwo(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--bogus"}); code != 2 {
		t.Errorf("exit %d", code)
	}
	if !strings.HasPrefix(h.stderr.String(), "Unknown argument: --bogus\nUsage:") {
		t.Errorf("stderr %q", h.stderr.String())
	}
}

func TestMissingValue(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--cancel"}); code != 1 || h.stderr.String() != "Missing value for --cancel.\n" {
		t.Errorf("exit %d stderr %q", code, h.stderr.String())
	}
}

func TestDryRunSingleMatch(t *testing.T) {
	h := newHarness(t, nil)
	code := h.a.main([]string{"--kind", "codex", "--at", "16:30", "--message", "hi", "--dry-run"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if h.stderr.String() != "Only one target matched; selecting codex in wB:p3 (idle, ~/projects/shop-backend).\n" {
		t.Errorf("stderr %q", h.stderr)
	}
	for _, want := range []string{
		"target=agent wB:p3 (codex, idle)\n", "at=2026-10-03 16:30:00\n", "delay_seconds=4080\n",
		"id=20261003-163000-wBp3\n", "would_run=/opt/schedule-send __fire 20261003-163000-wBp3\n",
	} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
}

func TestAmbiguousWithoutTerminalListsTargets(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--at", "16:30", "--message", "hi"}); code != 1 {
		t.Errorf("exit %d", code)
	}
	for _, want := range []string{
		"Multiple targets matched.",
		"  2  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)\n",
		"  5  · zsh      wB:p5",
	} {
		if !strings.Contains(h.stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, h.stderr)
		}
	}
}

func TestPickBestWithoutAtTakesTheLimitedAgent(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pick", "best", "--auto", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.HasPrefix(h.stderr.String(), "Multiple targets matched; picking claude in wB:p1 (idle, ~/projects/shop-backend).\n") {
		t.Errorf("stderr:\n%s", h.stderr)
	}
	if !strings.Contains(h.stdout.String(), "at=2026-10-03 16:01:00\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestPickerAndConfirmOnTerminal(t *testing.T) {
	// Without --at the limited agent comes first, so Enter takes it.
	input := "\n\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
┌  Schedule a send · dry run                Ctrl-C cancels
│
◆  Which agent or pane?
│  agents
│    1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
│    2  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite
│    3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
│    4  ● working  cursor   wE:p1    ~/src/schedule-send        Tidy schedule-send UI
│
│  panes
│    5  · zsh      wB:p5    ~/projects/shop-backend
│
│  Enter takes 1, or type its number
│  › │
│  claude · wB:p1 · ~/projects/shop-backend
│  ◷ hit its weekly limit · resets Sat 16:00, in 38m
│
◆  What should it send?
│  Enter keeps "resume"
│  › │
◆  When?
│  Enter keeps Sat 16:01, 1 min after the reset
│  › │
└  · Dry run: would schedule "resume" for Sat 16:01, in 39m
` + h.dryRunRows("20261003-160100-wBp1", "~/projects/shop-backend")
	if h.tty.String() != want {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wB:p1 (claude, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestDemoScenario(t *testing.T) {
	// What the README's recording shows. When this changes, run tools/media.
	input := "\n\n\n"
	h := newHarness(t, &input)
	h.useDemo(t)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
┌  Schedule a send · dry run                Ctrl-C cancels
│
◆  Which agent or pane?
│  agents
│    1  ◷ limited  claude   wA:p1    ~/projects/billing-api     Move invoices to the ledger  resets 16:00 (session)
│    2  ○ idle     codex    wA:p2    ~/projects/billing-api     Add retries to webhook delivery
│    3  ● working  codex    wB:p1    ~/projects/website         Redesign the pricing page
│
│  panes
│    4  · zsh      wA:p3    ~/projects/billing-api
│
│  Enter takes 1, or type its number
│  › │
│  claude · wA:p1 · ~/projects/billing-api
│  ◷ hit its session limit · resets Thu 16:00, in 2h 36m
│
◆  What should it send?
│  Enter keeps "resume"
│  › │
◆  When?
│  Enter keeps Thu 16:01, 1 min after the reset
│  › │
└  · Dry run: would schedule "resume" for Thu 16:01, in 2h 37m
` + h.dryRunRows("20261015-160100-wAp1", "~/projects/billing-api")
	if h.tty.String() != want {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "at=2026-10-15 16:01:00\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestPickerWithAtKeepsOrderAndMarks(t *testing.T) {
	input := "\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--at", "23:59", "--message", "x", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	// The picker is the only question, so the receipt follows it.
	want := `
┌  Schedule a send · dry run                Ctrl-C cancels
│
◆  Which agent or pane?
│  agents
│    1  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite
│    2  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
│    3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
│    4  ● working  cursor   wE:p1    ~/src/schedule-send        Tidy schedule-send UI
│
│  panes
│    5  · zsh      wB:p5    ~/projects/shop-backend
│
│  Enter takes 1, or type its number
│  › │
└  · Dry run: would schedule "x" for Sat 23:59, in 8h 37m
` + h.dryRunRows("20261003-235900-wAp1", "~/projects/analytics")
	if h.tty.String() != want {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wA:p1 (claude, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestPickerMarksAccountLimitedInPlace(t *testing.T) {
	input := "\n\n\n"
	h := newHarness(t, &input)
	h.edit(t, "visible-wA-p1.txt", " week:11% (1h58m)", "")
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
┌  Schedule a send · dry run                Ctrl-C cancels
│
◆  Which agent or pane?
│  agents
│    1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
│    2  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite  account limited
│    3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
`
	if !strings.HasPrefix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
}

func TestPickerReadsEachAgentScreenOnce(t *testing.T) {
	input := "\n\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	// The time prompt for wB:p1 reuses the picker's read; wB:p2 runs the
	// command, and wB:p5 is a plain pane.
	want := []string{
		"agent read wA:p1 --source visible --lines 120",
		"agent read wB:p1 --source visible --lines 120",
		"agent read wB:p3 --source visible --lines 120",
		"agent read wE:p1 --source visible --lines 120",
	}
	if got := h.reads(); !slices.Equal(got, want) {
		t.Errorf("reads:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPlainPaneWithAtReadsNoScreen(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pane", "wB:p5", "--at", "16:30", "--message", "ls", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if got := h.reads(); len(got) != 0 {
		t.Errorf("reads: %q", got)
	}
}

func TestAtBeforeTheResetWarns(t *testing.T) {
	cases := []struct {
		name    string
		account bool
		args    []string
		want    string
	}{
		{"own limit", false, []string{"--pane", "wB:p1", "--at", "15:30", "--message", "x"},
			"Warning: claude in wB:p1 is usage-limited until Sat 16:00; this send at Sat 15:30 comes before the reset.\n"},
		{"after the reset", false, []string{"--pane", "wB:p1", "--at", "16:30", "--message", "x"}, ""},
		{"keys", false, []string{"--pane", "wB:p1", "--at", "15:30", "--keys", "esc"}, ""},
		{"account", true, []string{"--pane", "wA:p1", "--at", "15:30", "--message", "x"},
			"Warning: claude's account looks usage-limited until Sat 16:00 (notice in wB:p1); this send to wA:p1 at Sat 15:30 comes before the reset.\n"},
		{"own gauge shows room", false, []string{"--pane", "wA:p1", "--at", "15:30", "--message", "x"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, nil)
			if c.account {
				h.edit(t, "visible-wA-p1.txt", " week:11% (1h58m)", "")
			}
			if code := h.a.main(append(c.args, "--dry-run")); code != 0 {
				t.Fatalf("exit %d: %s", code, h.stderr)
			}
			if h.stderr.String() != c.want {
				t.Errorf("stderr %q, want %q", h.stderr, c.want)
			}
		})
	}
}

func TestLimitNote(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	prev := when.Loc
	when.Loc = loc
	t.Cleanup(func() { when.Loc = prev })
	at := func(mo time.Month, d, hr, mi int) int64 { return time.Date(2026, mo, d, hr, mi, 0, 0, loc).Unix() }
	now := at(10, 3, 15, 22)

	cases := []struct {
		st   limit.Status
		want string
	}{
		{limit.Status{State: limit.Limited, Window: "weekly", Reset: at(10, 3, 16, 0)}, "resets 16:00 (weekly)"},
		{limit.Status{State: limit.Limited, Window: "weekly", Reset: at(10, 4, 16, 0), Unconfirmed: true}, "resets Sun 16:00 (weekly, unconfirmed)"},
		{limit.Status{State: limit.Limited, Reset: at(10, 9, 0, 27)}, "resets Fri 00:27"},
		{limit.Status{State: limit.Limited, Reset: at(10, 10, 0, 27)}, "resets Oct 10 00:27"},
		{limit.Status{State: limit.Limited}, "reset unknown"},
		{limit.Status{State: limit.Over, Window: "weekly", Reset: at(10, 3, 15, 0)}, "limit over since 15:00 (weekly)"},
		{limit.Status{State: limit.Over, Reset: at(9, 27, 0, 27)}, "limit over since Sep 27 00:27"},
		{limit.Status{State: limit.Over, Window: "session"}, "limit over (session)"},
	}
	for _, c := range cases {
		if got := limitNote(c.st, now); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestAutoAfterTheResetSendsAMinuteFromNow(t *testing.T) {
	h := newHarness(t, nil)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1791037000") // 16:16:40
	if code := h.a.main([]string{"--pane", "wB:p1", "--auto", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
		"That reset passed at Sat 16:00; sending \"resume\" 1 min from now.\n"
	if h.stderr.String() != want {
		t.Errorf("stderr %q, want %q", h.stderr, want)
	}
	if !strings.Contains(h.stdout.String(), "at=2026-10-03 16:17:40\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestAutoUnconfirmedResetSaysSo(t *testing.T) {
	h := newHarness(t, nil)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1791037000") // 16:16:40
	h.edit(t, "visible-wB-p1.txt", " week:0% (44m)", "")
	if code := h.a.main([]string{"--pane", "wB:p1", "--auto", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
		"Resets Sun 16:00 (in 23h 44m); sending \"resume\" 1 min after.\n" +
		"The notice names no day; if it was shown before Sat 16:00, the limit is already over: rerun with --at '+1 min'.\n"
	if h.stderr.String() != want {
		t.Errorf("stderr %q, want %q", h.stderr, want)
	}
	if !strings.Contains(h.stdout.String(), "at=2026-10-04 16:01:00\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestConfirmUnconfirmedResetSaysTypeOneMinute(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1791037000") // 16:16:40
	h.edit(t, "visible-wB-p1.txt", " week:0% (44m)", "")
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "│    the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min\n│\n" +
		"◆  What should it send?\n│  Enter keeps \"resume\"\n│  › │\n◆  When?\n│  Enter keeps Sun 16:01, 1 min after the reset\n"
	if !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
	}
}

func TestConfirmOverDefaultStaysAMinuteAhead(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	h.a.clock = waiting(1791037000, 120) // 16:16:40, Enter at 16:18:40
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "│  Enter keeps Sat 16:17:40, 1 min from now\n"; !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
	}
	for _, want := range []string{"at=2026-10-03 16:19:40\n", "delay_seconds=60\n"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
}

func TestConfirmResetDefaultStaysAsShown(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	h.a.clock = waiting(1791033720, 120) // 15:22, Enter at 15:24
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	for _, want := range []string{"at=2026-10-03 16:01:00\n", "delay_seconds=2220\n"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
}

func TestConfirmJudgesAnswersWhenGiven(t *testing.T) {
	t.Run("typed +1 min", func(t *testing.T) {
		input := "\n+1 min\n\n"
		h := newHarness(t, &input)
		// date(1) resolves "+1 min" from the real clock, so the answer is
		// given at the real time.
		answered := time.Now().Unix()
		h.a.clock = waiting(answered-120, 120)
		if code := h.a.main([]string{"--pane", "wB:p3", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		_, rest, _ := strings.Cut(h.stdout.String(), "run_at_epoch=")
		line, _, _ := strings.Cut(rest, "\n")
		runAt, _ := strconv.ParseInt(line, 10, 64)
		if d := runAt - answered; d < 60 || d > 65 {
			t.Fatalf("sends %d s after the answer, want about 60:\n%s", d, h.stdout)
		}
		if want := " · in " + when.Delay(runAt-answered) + " · Enter keeps it\n"; !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal lacks %q:\n%s", want, h.tty)
		}
	})
	t.Run("a time passed while the prompt waited", func(t *testing.T) {
		input := "\n2026-10-03 16:18\n16:20\n\n"
		h := newHarness(t, &input)
		h.a.clock = waiting(1791037000, 120) // 16:16:40, answered at 16:18:40
		if code := h.a.main([]string{"--pane", "wB:p3", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		for _, want := range []string{"│  ✗ Sat 16:18 has already passed.\n", "│  → Sat 16:20 · in 2m · Enter keeps it\n"} {
			if !strings.Contains(h.tty.String(), want) {
				t.Errorf("terminal lacks %q:\n%s", want, h.tty)
			}
		}
		if !strings.Contains(h.stdout.String(), "at=2026-10-03 16:20:00\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
	})
}

func TestReceiptOnATerminal(t *testing.T) {
	h := newHarness(t, nil)
	h.a.stdoutTTY = terminal
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "two\nlines"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
  ✓ scheduled "two" (+1 line) for codex wB:p3
    at       Sat 16:30   in 1h 8m
    cancel   schedule-send --cancel 20261003-163000-wBp3
`
	if h.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", h.stdout, want)
	}
}

func TestDryRunOnATerminal(t *testing.T) {
	h := newHarness(t, nil)
	h.a.stdoutTTY = terminal
	if code := h.a.main([]string{"--kind", "codex", "--at", "16:30", "--message", "hi", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
  · dry run: would schedule "hi" for codex wB:p3
    at       Sat 16:30   in 1h 8m
    cwd      ~/projects/shop-backend
    if-busy  wait
    recover  keys
    grace    4s
    persist  no
    id       20261003-163000-wBp3
    units    ` + h.a.sys.Files(h.a.paths, "20261003-163000-wBp3", false) + `
    runs     /opt/schedule-send __fire 20261003-163000-wBp3
`
	if h.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", h.stdout, want)
	}
	if h.stderr.String() != "Only one target matched; selecting codex in wB:p3 (idle, ~/projects/shop-backend).\n" {
		t.Errorf("stderr %q", h.stderr)
	}
}

func TestAutoReportOnAStderrTerminal(t *testing.T) {
	t.Run("styled", func(t *testing.T) {
		h := newHarness(t, nil)
		h.a.stderrTTY = terminal
		if code := h.a.main([]string{"--pane", "wB:p1", "--auto", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		want := `
  ◷ claude wB:p1 hit its weekly limit
    resets   Sat 16:00   in 38m
    send     "resume"    1 min after
`
		if h.stderr.String() != want {
			t.Errorf("stderr:\n%s\nwant:\n%s", h.stderr, want)
		}
		if !strings.Contains(h.stdout.String(), "at=2026-10-03 16:01:00\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
	})
	t.Run("its failure stays plain", func(t *testing.T) {
		h := newHarness(t, nil)
		h.a.stderrTTY = terminal
		if code := h.a.main([]string{"--pane", "wB:p3", "--auto", "--dry-run"}); code != 1 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		want := "No usage-limit notice found for codex in wB:p3 (idle, ~/projects/shop-backend).\n" +
			"--auto needs a notice that names its reset time; pass --at instead.\n"
		if h.stderr.String() != want {
			t.Errorf("stderr %q, want %q", h.stderr, want)
		}
	})
}

func TestJSONDryRunAndNote(t *testing.T) {
	h := newHarness(t, nil)
	h.a.stdoutTTY = terminal
	if code := h.a.main([]string{"--kind", "codex", "--at", "16:30", "--message", "hi", "--dry-run", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got := jsonObject(t, h.stdout.Bytes())
	if got["dry_run"] != true || got["id"] != "20261003-163000-wBp3" || got["delay_seconds"] != 4080.0 {
		t.Errorf("stdout: %s", h.stdout)
	}
	tgt := got["target"].(map[string]any)
	if tgt["pane"] != "wB:p3" || tgt["agent"] != "codex" {
		t.Errorf("target: %v", tgt)
	}
	if _, ok := got["limit"]; ok {
		t.Errorf("codex has a limit: %s", h.stdout)
	}
	note := jsonObject(t, h.stderr.Bytes())
	if note["note"] != "Only one target matched; selecting codex in wB:p3 (idle, ~/projects/shop-backend)." {
		t.Errorf("stderr: %s", h.stderr)
	}
}

func TestJSONStillAsks(t *testing.T) {
	input := "\n\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.Contains(h.tty.String(), "│  ◷ hit its weekly limit · resets Sat 16:00, in 38m\n") ||
		!strings.Contains(h.tty.String(), "│  Enter keeps Sat 16:01, 1 min after the reset\n") {
		t.Errorf("terminal:\n%s", h.tty)
	}
	got := jsonObject(t, h.stdout.Bytes())
	lim := got["limit"].(map[string]any)
	if lim["state"] != "limited" || lim["window"] != "weekly" || lim["reset"] != 1791036000.0 || lim["note"] != "resets 16:00 (weekly)" {
		t.Errorf("limit: %v", lim)
	}
	if got["at"] != "2026-10-03 16:01:00" {
		t.Errorf("at: %v", got["at"])
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr: %s", h.stderr)
	}
}

func TestJSONErrors(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--json", "--bogus"}); code != 2 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got := jsonObject(t, h.stderr.Bytes())
	err := got["error"].(map[string]any)
	if err["code"] != "usage" || err["message"] != "Unknown argument: --bogus" {
		t.Errorf("stderr: %s", h.stderr)
	}
	if strings.Contains(h.stderr.String(), "Usage:") {
		t.Errorf("usage text beside the error: %s", h.stderr)
	}

	h = newHarness(t, nil)
	if code := h.a.main([]string{"--at", "16:30", "--message", "hi", "--json"}); code != 1 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got = jsonObject(t, h.stderr.Bytes())
	err = got["error"].(map[string]any)
	if err["code"] != "ambiguous" || !strings.HasPrefix(err["message"].(string), "Multiple targets matched.") {
		t.Errorf("stderr: %s", h.stderr)
	}
}

func TestJSONWarning(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pane", "wB:p1", "--at", "15:30", "--message", "x", "--dry-run", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got := jsonObject(t, h.stderr.Bytes())
	if got["warning"] != "Warning: claude in wB:p1 is usage-limited until Sat 16:00; this send at Sat 15:30 comes before the reset." {
		t.Errorf("stderr: %s", h.stderr)
	}
	if jsonObject(t, h.stdout.Bytes())["dry_run"] != true {
		t.Errorf("stdout: %s", h.stdout)
	}
}

func TestPickerEOFDoesNotChooseTarget(t *testing.T) {
	input := ""
	h := newHarness(t, &input)
	code := h.a.main([]string{"--at", "16:30", "--message", "hi", "--dry-run", "--json"})
	if code != 1 || h.stdout.Len() != 0 {
		t.Fatalf("exit %d stdout %s", code, h.stdout)
	}
	if !strings.Contains(h.stderr.String(), "Target selection cancelled") {
		t.Fatalf("stderr %s", h.stderr)
	}
}

func TestReceiptIntoAPipeStaysKeyValue(t *testing.T) {
	// Every stream but stdout is a terminal.
	input := "\n\n"
	h := newHarness(t, &input)
	h.a.stderrTTY = terminal
	if code := h.a.main([]string{"--pane", "wB:p1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `id=20261003-160100-wBp1
unit=` + h.a.sys.Name("20261003-160100-wBp1") + `
target=agent wB:p1 (claude, idle)
at=2026-10-03 16:01:00
delay_seconds=2340
payload=resume

cancel with: schedule-send --cancel 20261003-160100-wBp1
`
	if h.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", h.stdout, want)
	}
}

func TestEndOfInputEndsTheRail(t *testing.T) {
	input := "\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 1 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "│  › └  ✗ No send time given.\n"; !strings.HasSuffix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to end:\n%s", h.tty, want)
	}
	if h.stderr.Len() != 0 || h.stdout.Len() != 0 {
		t.Errorf("stderr %q, stdout %q", h.stderr, h.stdout)
	}
}

func TestRailEndTakesTheStyledReceiptsPlace(t *testing.T) {
	// stdout is the terminal the rail is drawn on, so the receipt is not
	// printed twice.
	input := "\n\n"
	h := newHarness(t, &input)
	h.a.stdoutTTY = terminal
	if code := h.a.main([]string{"--pane", "wB:p1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "└  ✓ Scheduled \"resume\" for Sat 16:01, in 39m\n   cancel   schedule-send --cancel 20261003-160100-wBp1\n"
	if !strings.HasSuffix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to end:\n%s", h.tty, want)
	}
	if h.stdout.Len() != 0 {
		t.Errorf("stdout %q", h.stdout)
	}
}

func TestWarningsGoOnTheRail(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	h.a.installedPath = func() (string, error) { return "", errors.New("not installed") }
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "│  Warning: this build cannot arm jobs (not installed)"; !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr %q", h.stderr)
	}
}

func TestMessageQuestion(t *testing.T) {
	t.Run("typed on an agent", func(t *testing.T) {
		input := "run the e2e suite\n\n"
		h := newHarness(t, &input)
		if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if !strings.Contains(h.stdout.String(), "payload=run the e2e suite\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
		if want := `└  · Dry run: would schedule "run the e2e suite" for Sat 16:01`; !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
		}
	})
	t.Run("a shell pane has to be told", func(t *testing.T) {
		// Enter and blanks are asked again; then the time, which a pane has
		// no notice to propose.
		input := "\n  \nls -la\n17:00\n\n"
		h := newHarness(t, &input)
		if code := h.a.main([]string{"--pane", "wB:p5", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		want := "│  pane wB:p5 · ~/projects/shop-backend\n│\n" +
			"◆  What should it send?\n│  It's a shell, so this runs as a command\n│  › │  › │  › │\n" +
			"◆  When?\n│  Like 18:30, 6:30 PM, Sep 27 1am or +30 min\n│  › │  → Sat 17:00 · in 1h 38m · Enter keeps it\n│  › │\n"
		if !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
		}
		if !strings.Contains(h.stdout.String(), "payload=ls -la\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
	})
	t.Run("end of input", func(t *testing.T) {
		input := ""
		h := newHarness(t, &input)
		if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 1 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if want := "│  › └  ✗ No message given.\n"; !strings.HasSuffix(h.tty.String(), want) {
			t.Errorf("terminal:\n%s\nwant it to end:\n%s", h.tty, want)
		}
	})
	t.Run("not asked when given", func(t *testing.T) {
		for _, args := range [][]string{{"--message", "hi"}, {"--keys", "esc"}} {
			input := "\n"
			h := newHarness(t, &input)
			if code := h.a.main(append([]string{"--pane", "wB:p1", "--dry-run"}, args...)); code != 0 {
				t.Fatalf("%v: exit %d: %s", args, code, h.stderr)
			}
			if strings.Contains(h.tty.String(), "What should it send?") {
				t.Errorf("%v asked:\n%s", args, h.tty)
			}
		}
	})
	t.Run("a shell pane under --auto is still refused", func(t *testing.T) {
		h := newHarness(t, nil)
		if code := h.a.main([]string{"--pane", "wB:p5", "--auto", "--dry-run"}); code != 1 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if want := "Pane wB:p5 is a plain shell, where the default 'resume' would run as a command. Pass --message.\n"; h.stderr.String() != want {
			t.Errorf("stderr %q, want %q", h.stderr, want)
		}
	})
}

func TestPickerAsksAgainOnTheRail(t *testing.T) {
	input := "9\nx\n3\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--at", "23:59", "--message", "x", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "│  › │  ✗ Enter a number from 1 to 5.\n│  › │  ✗ Enter a number from 1 to 5.\n│  › │\n└  · Dry run: would schedule \"x\""
	if !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to hold:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wB:p3 (codex, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestShortOffsetAndStrayKeyCodes(t *testing.T) {
	// The arrow keys' codes come through a terminal that reads whole lines,
	// and are left out of the answer; "+30" is thirty minutes.
	input := "\n\x1b[A\x1b[C+30\n\n"
	h := newHarness(t, &input)
	// date(1) resolves "+30" from the real clock, so the answer is given at
	// the real time.
	answered := time.Now().Unix()
	h.a.clock = waiting(answered-120, 120)
	if code := h.a.main([]string{"--pane", "wB:p3", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	_, rest, _ := strings.Cut(h.stdout.String(), "run_at_epoch=")
	line, _, _ := strings.Cut(rest, "\n")
	runAt, _ := strconv.ParseInt(line, 10, 64)
	if d := runAt - answered; d < 1800 || d > 1805 {
		t.Fatalf("sends %d s after the answer, want about 1800:\n%s", d, h.stdout)
	}
	if want := " · in " + when.Delay(runAt-answered) + " · Enter keeps it\n"; !strings.Contains(h.tty.String(), want) {
		t.Errorf("terminal lacks %q:\n%s", want, h.tty)
	}
	if strings.Contains(h.tty.String(), "\x1b") || strings.Contains(h.tty.String(), "Could not read") {
		t.Errorf("terminal:\n%q", h.tty)
	}
}

func TestCancelledEndsTheRail(t *testing.T) {
	h := newHarness(t, nil)
	h.a.ask = &asker{tty: fakeTTY{strings.NewReader(""), h.tty}, rail: show.NewRail(term.Colors{})}
	var f failure
	if !errors.As(cancelled(), &f) || f.code != 130 {
		t.Fatalf("cancelled() = %#v", cancelled())
	}
	h.a.failed(f)
	if want := "└  Cancelled. Nothing was scheduled.\n"; h.tty.String() != want {
		t.Errorf("terminal %q, want %q", h.tty, want)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr %q", h.stderr)
	}
}
