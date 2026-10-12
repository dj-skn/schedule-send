package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"schedule-send/internal/job"
	"schedule-send/internal/limit"
	"schedule-send/internal/when"
)

// fakeTTY is a terminal fed fixed input, recording what was drawn on it.
type fakeTTY struct {
	in  *strings.Reader
	out *bytes.Buffer
}

func (f fakeTTY) Read(p []byte) (int, error)  { return f.in.Read(p) }
func (f fakeTTY) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f fakeTTY) Close() error                { return nil }

type harness struct {
	a              *app
	stdout, stderr *bytes.Buffer
	tty            *bytes.Buffer
	dir            string
}

// newHarness runs the command against the stand-in herdr and systemctl, at
// 15:22 on 2026-10-03 in Warsaw, from pane wB:p2.
func newHarness(t *testing.T, ttyInput *string) *harness {
	t.Helper()
	root, _ := filepath.Abs("../..")
	dir := t.TempDir()
	herdrDir := filepath.Join(dir, "herdr")
	os.MkdirAll(herdrDir, 0o755)
	fixtures := filepath.Join(root, "testdata/fixtures/2026-10-03")
	entries, _ := os.ReadDir(fixtures)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(fixtures, e.Name()))
		os.WriteFile(filepath.Join(herdrDir, e.Name()), b, 0o644)
	}
	path := filepath.Join(dir, "path")
	os.MkdirAll(path, 0o755)
	os.Symlink(filepath.Join(root, "testdata/standin/systemctl"), filepath.Join(path, "systemctl"))
	os.Symlink(filepath.Join(root, "testdata/standin/journalctl"), filepath.Join(path, "journalctl"))

	t.Setenv("PATH", path+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_BIN_OVERRIDE", filepath.Join(root, "testdata/standin/herdr"))
	t.Setenv("HERDR_STANDIN_DIR", herdrDir)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1791033720")
	t.Setenv("HOME", "/home/dev")
	t.Setenv("TZ", "Europe/Warsaw")
	t.Setenv("NO_COLOR", "1")
	loc, _ := time.LoadLocation("Europe/Warsaw")
	prev := when.Loc
	when.Loc = loc
	t.Cleanup(func() { when.Loc = prev })

	h := &harness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, tty: &bytes.Buffer{}, dir: dir}
	h.a = &app{
		stdout:    h.stdout,
		stderr:    h.stderr,
		stdinTTY:  func() bool { return ttyInput != nil },
		stdoutTTY: func() bool { return false },
		stderrTTY: func() bool { return false },
		openTTY: func() (io.ReadWriteCloser, error) {
			return fakeTTY{strings.NewReader(*ttyInput), h.tty}, nil
		},
		paths:         job.Paths{StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "units")},
		selfPane:      "wB:p2",
		self:          "/opt/schedule-send",
		installedPath: func() (string, error) { return "/opt/schedule-send", nil },
	}
	return h
}

// edit rewrites one of the stand-in's files, failing the test when old is not
// in it.
func (h *harness) edit(t *testing.T, name, old, with string) {
	t.Helper()
	p := filepath.Join(h.dir, "herdr", name)
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s lacks %q", name, old)
	}
	os.WriteFile(p, []byte(strings.Replace(string(b), old, with, 1)), 0o644)
}

// waiting is a clock that reads start at the start of the run and start+wait
// on every later reading, as if the time prompt sat unanswered for wait
// seconds.
func waiting(start, wait int64) func() int64 {
	read := false
	return func() int64 {
		if !read {
			read = true
			return start
		}
		return start + wait
	}
}

// reads is every screen read the stand-in served, sorted.
func (h *harness) reads() []string {
	b, _ := os.ReadFile(filepath.Join(h.dir, "herdr", "calls.log"))
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "agent read ") {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

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
	input := "\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
agents
  1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
  2  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite
  3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
  4  ● working  cursor   wE:p1    ~/src/schedule-send        Tidy schedule-send UI

panes
  5  · zsh      wB:p5    ~/projects/shop-backend

target [1]: 
  ◷ claude wB:p1 hit its weekly limit
    resets   Sat 16:00   in 38m
    send     "resume"    1 min after

  at [Sat 16:01]: `
	if h.tty.String() != want {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wB:p1 (claude, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestPickerWithAtKeepsOrderAndMarks(t *testing.T) {
	input := "\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--at", "23:59", "--message", "x", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
agents
  1  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite
  2  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
  3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
  4  ● working  cursor   wE:p1    ~/src/schedule-send        Tidy schedule-send UI
`
	if !strings.HasPrefix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
	if !strings.Contains(h.stdout.String(), "target=agent wA:p1 (claude, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func TestPickerMarksAccountLimitedInPlace(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	h.edit(t, "visible-wA-p1.txt", " week:11% (1h58m)", "")
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
agents
  1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
  2  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite  account limited
  3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend
`
	if !strings.HasPrefix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant:\n%s", h.tty, want)
	}
}

func TestPickerReadsEachAgentScreenOnce(t *testing.T) {
	input := "\n\n"
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
	input := "\n"
	h := newHarness(t, &input)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1791037000") // 16:16:40
	h.edit(t, "visible-wB-p1.txt", " week:0% (44m)", "")
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := "    note     the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min\n\n  at [Sun 16:01]: "
	if !strings.HasSuffix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to end:\n%s", h.tty, want)
	}
}

func TestConfirmOverDefaultStaysAMinuteAhead(t *testing.T) {
	input := "\n"
	h := newHarness(t, &input)
	h.a.clock = waiting(1791037000, 120) // 16:16:40, Enter at 16:18:40
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "    send     \"resume\"    1 min from now\n\n  at [Sat 16:17:40]: "; !strings.HasSuffix(h.tty.String(), want) {
		t.Errorf("terminal:\n%s\nwant it to end:\n%s", h.tty, want)
	}
	for _, want := range []string{"at=2026-10-03 16:19:40\n", "delay_seconds=60\n"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
}

func TestConfirmResetDefaultStaysAsShown(t *testing.T) {
	input := "\n"
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
		input := "+1 min\n\n"
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
		if want := " in " + when.Delay(runAt-answered) + "\n"; !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal lacks %q:\n%s", want, h.tty)
		}
	})
	t.Run("a time passed while the prompt waited", func(t *testing.T) {
		input := "2026-10-03 16:18\n16:20\n\n"
		h := newHarness(t, &input)
		h.a.clock = waiting(1791037000, 120) // 16:16:40, answered at 16:18:40
		if code := h.a.main([]string{"--pane", "wB:p3", "--dry-run"}); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		for _, want := range []string{"    Sat 16:18 has already passed.\n", "    → Sat 16:20   in 2m\n"} {
			if !strings.Contains(h.tty.String(), want) {
				t.Errorf("terminal lacks %q:\n%s", want, h.tty)
			}
		}
		if !strings.Contains(h.stdout.String(), "at=2026-10-03 16:20:00\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
	})
}

func TestArmWritesStateAndUnits(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "two\nlines"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	id := "20261003-163000-wBp3"
	s, err := job.Load(h.a.paths.StatePath(id))
	if err != nil || s.PaneID != "wB:p3" || s.Mode != "text" || s.At != "2026-10-03 16:30:00" {
		t.Fatalf("state %+v, %v", s, err)
	}
	if msg, _ := os.ReadFile(h.a.paths.MessagePath(id)); string(msg) != "two\nlines" {
		t.Errorf("message %q", msg)
	}
	service, _ := os.ReadFile(h.a.paths.UnitPath(id, "service"))
	if !strings.Contains(string(service), `ExecStart=/usr/bin/env -- "/opt/schedule-send" __fire `+id+"\n") {
		t.Errorf("service:\n%s", service)
	}
	if !strings.HasSuffix(h.stdout.String(), "payload=two\n\ncancel with: schedule-send --cancel "+id+"\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}

func terminal() bool { return true }

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
    units    ` + filepath.Join(h.dir, "units") + `/herdr-send-20261003-163000-wBp3.{service,timer}
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

// addJobs stores two sends: "resume" to wB:p1 at 16:01, whose timer the
// stand-in reports active, and ctrl+c to the plain pane wB:p5 tomorrow.
func (h *harness) addJobs(t *testing.T) {
	t.Helper()
	msg := "resume"
	for _, j := range []struct {
		s   job.State
		msg *string
	}{
		{job.State{ID: "20261003-160100-wBp1", Kind: "agent", PaneID: "wB:p1", Agent: "claude", At: "2026-10-03 16:01:00", Mode: "text", Grace: 4}, &msg},
		{job.State{ID: "20261004-090000-wBp5", Kind: "pane", PaneID: "wB:p5", Agent: "-", At: "2026-10-04 09:00:00", Mode: "keys", Keys: "ctrl+c", Grace: 4}, nil},
	} {
		if err := h.a.paths.Write(j.s, j.msg); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SYSTEMCTL_STANDIN_ACTIVE", "herdr-send-20261003-160100-wBp1.timer")
}

func TestListOnATerminal(t *testing.T) {
	h := newHarness(t, nil)
	h.a.stdoutTTY = terminal
	h.addJobs(t)
	if code := h.a.main([]string{"--list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `
  ○ Sat 16:01   in 39m    claude wB:p1   "resume"                   20261003-160100-wBp1
  ✗ Sun 09:00   inactive  pane wB:p5     keys: ctrl+c               20261004-090000-wBp5
`
	if h.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", h.stdout, want)
	}

	t.Run("a bad clock override falls back to the real time", func(t *testing.T) {
		h := newHarness(t, nil)
		h.a.stdoutTTY = terminal
		h.addJobs(t)
		t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "soon")
		if code := h.a.main([]string{"--list"}); code != 0 || h.stderr.Len() != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if !strings.Contains(h.stdout.String(), " 20261003-160100-wBp1\n") {
			t.Errorf("stdout:\n%s", h.stdout)
		}
	})
}

func TestCancelOnATerminal(t *testing.T) {
	h := newHarness(t, nil)
	h.a.stdoutTTY = terminal
	h.addJobs(t)
	if code := h.a.main([]string{"--cancel", "20261003-160100-wBp1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if want := "  ✓ cancelled 20261003-160100-wBp1\n"; h.stdout.String() != want {
		t.Errorf("stdout %q, want %q", h.stdout, want)
	}
}

func TestReceiptIntoAPipeStaysKeyValue(t *testing.T) {
	// Every stream but stdout is a terminal.
	input := "\n"
	h := newHarness(t, &input)
	h.a.stderrTTY = terminal
	if code := h.a.main([]string{"--pane", "wB:p1"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	want := `id=20261003-160100-wBp1
unit=herdr-send-20261003-160100-wBp1.timer
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

func jsonObject(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, b)
	}
	return m
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
	input := "\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.Contains(h.tty.String(), "  ◷ claude wB:p1 hit its weekly limit\n") || !strings.Contains(h.tty.String(), "  at [Sat 16:01]: ") {
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

func TestJSONListCancelStatus(t *testing.T) {
	h := newHarness(t, nil)
	h.addJobs(t)
	if code := h.a.main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got := jsonObject(t, h.stdout.Bytes())
	jobs := got["jobs"].([]any)
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.(map[string]any)["id"].(string)] = true
	}
	if len(jobs) != 2 || !ids["20261003-160100-wBp1"] || !ids["20261004-090000-wBp5"] {
		t.Errorf("list: %s", h.stdout)
	}

	h = newHarness(t, nil)
	h.addJobs(t)
	if code := h.a.main([]string{"--cancel", "20261003-160100-wBp1", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if jsonObject(t, h.stdout.Bytes())["cancelled"] != "20261003-160100-wBp1" {
		t.Errorf("cancel: %s", h.stdout)
	}

	h = newHarness(t, nil)
	h.addJobs(t)
	if code := h.a.main([]string{"--status", "20261003-160100-wBp1", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got = jsonObject(t, h.stdout.Bytes())
	state := got["state"].(map[string]any)
	if state["id"] != "20261003-160100-wBp1" || got["timer"] != "stand-in status for herdr-send-20261003-160100-wBp1.timer" {
		t.Errorf("status: %s", h.stdout)
	}
}

func TestJSONTargets(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--targets", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	got := jsonObject(t, h.stdout.Bytes())
	rows := got["targets"].([]any)
	first := rows[0].(map[string]any)
	lim := first["limit"].(map[string]any)
	if first["pane"] != "wB:p1" || lim["state"] != "limited" || lim["reset"] != 1791036000.0 || lim["note"] != "resets 16:00 (weekly)" {
		t.Errorf("first: %v", first)
	}

	h = newHarness(t, nil)
	if code := h.a.main([]string{"--targets"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.Contains(h.stdout.String(), "  1  ◷ limited  claude   wB:p1") {
		t.Errorf("listing:\n%s", h.stdout)
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

func TestValidateAllArgumentsBeforeCancel(t *testing.T) {
	for _, tail := range [][]string{{"--bogus"}, {"--list"}, {"--dry-run"}} {
		t.Run(strings.Join(tail, " "), func(t *testing.T) {
			h := newHarness(t, nil)
			h.addJobs(t)
			id := "20261003-160100-wBp1"
			args := append([]string{"--cancel", id, "--json"}, tail...)
			if code := h.a.main(args); code == 0 {
				t.Fatalf("invalid command succeeded")
			}
			if _, err := os.Stat(h.a.paths.StatePath(id)); err != nil {
				t.Fatalf("cancelled before validation: %v", err)
			}
			if jsonObject(t, h.stderr.Bytes())["error"].(map[string]any)["code"] != "usage" {
				t.Fatalf("stderr %s", h.stderr)
			}
		})
	}
}

func TestListDoesNotRequireHerdr(t *testing.T) {
	h := newHarness(t, nil)
	t.Setenv("HERDR_BIN_OVERRIDE", filepath.Join(h.dir, "nonexistent-herdr"))
	if code := h.a.main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, h.stderr)
	}
	if len(jsonObject(t, h.stdout.Bytes())["jobs"].([]any)) != 0 {
		t.Fatalf("stdout %s", h.stdout)
	}
}

func TestDiscoveryErrorsHaveJSONDiagnostics(t *testing.T) {
	for _, operation := range [][]string{{"--targets"}, {"--pane", "wB:p3", "--at", "16:30", "--message", "hi", "--dry-run"}} {
		t.Run(operation[0], func(t *testing.T) {
			h := newHarness(t, nil)
			os.WriteFile(filepath.Join(h.dir, "herdr", "agent-list.json"), []byte("malformed JSON"), 0600)
			args := append(operation, "--json")
			if code := h.a.main(args); code != 1 {
				t.Fatalf("exit %d stderr %s", code, h.stderr)
			}
			err := jsonObject(t, h.stderr.Bytes())["error"].(map[string]any)
			if !strings.Contains(err["message"].(string), "Could not discover Herdr targets") {
				t.Fatalf("stderr %s", h.stderr)
			}
		})
	}
}

func TestDiscoveryCapturesChildStderr(t *testing.T) {
	h := newHarness(t, nil)
	script := filepath.Join(h.dir, "failing-herdr")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'lost connection to Herdr' >&2\nexit 1\n"), 0700)
	t.Setenv("HERDR_BIN_OVERRIDE", script)
	if code := h.a.main([]string{"--targets", "--json"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	err := jsonObject(t, h.stderr.Bytes())["error"].(map[string]any)
	if !strings.Contains(err["message"].(string), "lost connection to Herdr") {
		t.Fatalf("stderr %s", h.stderr)
	}
}

func TestListSkipsCorruptStateWithDiagnostic(t *testing.T) {
	h := newHarness(t, nil)
	h.addJobs(t)
	os.WriteFile(h.a.paths.StatePath("bad-job"), []byte("corrupt"), 0600)
	if code := h.a.main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, h.stderr)
	}
	if len(jsonObject(t, h.stdout.Bytes())["jobs"].([]any)) != 2 {
		t.Fatalf("stdout %s", h.stdout)
	}
	if !strings.Contains(jsonObject(t, h.stderr.Bytes())["warning"].(string), "bad-job.json") {
		t.Fatalf("stderr %s", h.stderr)
	}
}

func TestListUsesStoredEpoch(t *testing.T) {
	h := newHarness(t, nil)
	h.addJobs(t)
	id := "20261003-160100-wBp1"
	p := h.a.paths.StatePath(id)
	b, _ := os.ReadFile(p)
	v := jsonObject(t, b)
	v["run_at"] = int64(1791036000)
	b, _ = json.Marshal(v)
	os.WriteFile(p, b, 0600)
	when.Loc = time.UTC
	if code := h.a.main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, h.stderr)
	}
	for _, v := range jsonObject(t, h.stdout.Bytes())["jobs"].([]any) {
		row := v.(map[string]any)
		if row["id"] == id && row["run_at_epoch"] != float64(1791036000) {
			t.Fatalf("epoch changed with timezone: %v", row)
		}
	}
}

func TestEpochOverrideOverflowRejected(t *testing.T) {
	h := newHarness(t, nil)
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "9999999999999999999999999999999999")
	if code := h.a.main([]string{"--targets", "--json"}); code != 1 {
		t.Fatalf("exit %d stderr %s", code, h.stderr)
	}
	if !strings.Contains(jsonObject(t, h.stderr.Bytes())["error"].(map[string]any)["message"].(string), "in range") {
		t.Fatalf("stderr %s", h.stderr)
	}
}

func TestUnmanagedSchedulingRefusedBeforePublication(t *testing.T) {
	h := newHarness(t, nil)
	h.a.installedPath = func() (string, error) { return "", errors.New("this build has no installation path") }
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(h.stderr.String(), "Run tools/install from the main checkout, then") {
		t.Fatalf("stderr: %s", h.stderr)
	}
	for _, dir := range []string{h.a.paths.StateDir, h.a.paths.UnitDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("created job directory %s: %v", dir, err)
		}
	}
}

func TestInvalidServiceMetadataRefusedBeforePublication(t *testing.T) {
	h := newHarness(t, nil)
	h.a.installedPath = func() (string, error) { return "/opt/bad\npath", nil }
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(h.stderr.String(), "control character") {
		t.Fatalf("stderr: %s", h.stderr)
	}
	for _, dir := range []string{h.a.paths.StateDir, h.a.paths.UnitDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("created job directory %s: %v", dir, err)
		}
	}
}

// Exercise the real linked metadata and CLI with isolated Herdr/systemd stand-ins.
func TestBuiltInstallationAndUnmanagedCommands(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, nil)
	stateRoot := filepath.Join(h.dir, "state-root")
	configRoot := filepath.Join(h.dir, "config-root")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("HERDR_PANE_ID", "wB:p2")
	final := filepath.Join(h.dir, `installed space % $ " ' \`)
	staged := filepath.Join(h.dir, "staged")
	out, err := exec.Command(filepath.Join(root, "tools/build"), staged, final).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture build: %v: %s", err, out)
	}
	if err := os.Rename(staged, final); err != nil {
		t.Fatal(err)
	}
	run := func(bin string, args ...string) (string, error) {
		t.Helper()
		out, err := exec.Command(bin, args...).CombinedOutput()
		return string(out), err
	}
	schedule := []string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}
	copied := filepath.Join(h.dir, "copied")
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, data, 0700); err != nil {
		t.Fatal(err)
	}
	for _, bin := range []string{staged, copied} {
		if bin == staged {
			if err := os.Rename(final, staged); err != nil {
				t.Fatal(err)
			}
		}
		out, err := run(bin, schedule...)
		if err == nil || !strings.Contains(out, "Cannot arm a job:") {
			t.Fatalf("unmanaged schedule: %v: %s", err, out)
		}
		if bin == staged {
			if err := os.Rename(staged, final); err != nil {
				t.Fatal(err)
			}
		}
		for _, root := range []string{filepath.Join(stateRoot, "herdr-send"), filepath.Join(configRoot, "systemd/user")} {
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("created directory on refusal: %s: %v", root, err)
			}
		}
		if bin == copied {
			if out, err := run(bin, "--list", "--json"); err != nil {
				t.Fatalf("management: %v: %s", err, out)
			}
		}
	}
	if out, err := run(copied, append(schedule, "--dry-run")...); err != nil {
		t.Fatalf("copied preview: %v: %s", err, out)
	}
	plain := filepath.Join(h.dir, "plain")
	cmd := exec.Command("go", "build", "-o", plain, "./cmd/schedule-send")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plain fixture build: %v: %s", err, out)
	}
	if out, err := run(plain, schedule...); err == nil || !strings.Contains(out, "no installation path") {
		t.Fatalf("plain schedule: %v: %s", err, out)
	}
	if out, err := run(plain, append(schedule, "--dry-run")...); err != nil {
		t.Fatalf("plain preview: %v: %s", err, out)
	}
	if out, err := run(plain, "--list", "--json"); err != nil {
		t.Fatalf("plain management: %v: %s", err, out)
	}
	if out, err := run(plain, "__fire", "missing"); err == nil || !strings.Contains(out, "No state file") || strings.Contains(out, "installation") {
		t.Fatalf("legacy fire: %v: %s", err, out)
	}
	linked := filepath.Join(h.dir, "linked")
	if err := os.Symlink(final, linked); err != nil {
		t.Fatal(err)
	}
	if out, err := run(linked, schedule...); err != nil {
		t.Fatalf("installed schedule: %v: %s", err, out)
	}
	id := "20261003-163000-wBp3"
	unit, err := os.ReadFile(filepath.Join(configRoot, "systemd/user/herdr-send-"+id+".service"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := job.ServiceUnit("wB:p3", "2026-10-03 16:30:00", final, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(unit) != expected {
		t.Fatalf("service %q, want %q", unit, expected)
	}
}

func TestMalformedNumericStateRemainsInspectable(t *testing.T) {
	for _, operation := range []string{"--list", "--status"} {
		t.Run(operation, func(t *testing.T) {
			h := newHarness(t, nil)
			h.addJobs(t)
			id := "20261003-160100-wBp1"
			data, err := os.ReadFile(h.a.paths.StatePath(id))
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte(`"grace": 4`), []byte(`"grace": 4.0`), 1)
			if err := os.WriteFile(h.a.paths.StatePath(id), data, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{operation, "--json"}
			if operation == "--status" {
				args = []string{operation, id, "--json"}
			}
			if code := h.a.main(args); code != 0 {
				t.Fatalf("exit %d: %s", code, h.stderr)
			}
			if operation == "--list" {
				jobs := jsonObject(t, h.stdout.Bytes())["jobs"].([]any)
				if len(jobs) != 1 {
					t.Fatalf("jobs: %s", h.stdout)
				}
				if jobs[0].(map[string]any)["id"] == id {
					t.Fatal("listed malformed job")
				}
				warning := jsonObject(t, h.stderr.Bytes())["warning"].(string)
				if !strings.Contains(warning, "job field grace") {
					t.Fatalf("warning: %s", warning)
				}
			} else {
				var result map[string]json.RawMessage
				if err := json.Unmarshal(h.stdout.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				var state map[string]json.RawMessage
				if err := json.Unmarshal(result["state"], &state); err != nil {
					t.Fatal(err)
				}
				if string(state["grace"]) != "4.0" {
					t.Fatalf("state lost invalid numeric syntax: %s", result["state"])
				}
			}
			stored, err := os.ReadFile(h.a.paths.StatePath(id))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stored, data) {
				t.Fatal("inspection changed malformed state")
			}
		})
	}
}
