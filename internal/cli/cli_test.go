package cli

import (
	"bytes"
	"io"
	"os"
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
		stdout:   h.stdout,
		stderr:   h.stderr,
		stdinTTY: func() bool { return ttyInput != nil },
		openTTY: func() (io.ReadWriteCloser, error) {
			return fakeTTY{strings.NewReader(*ttyInput), h.tty}, nil
		},
		paths:    job.Paths{StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "units")},
		selfPane: "wB:p2",
		self:     "/opt/schedule-send",
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
Usage limit in claude wB:p1:
  resets 4pm (Europe/Warsaw)
Resets Sat 16:00 (in 38m); sending "resume" 1 min after.

at [2026-10-03 16:01]: `
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
	want := "The notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min.\n\nat [2026-10-04 16:01]: "
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
	if want := "sending \"resume\" 1 min from now.\n\nat [2026-10-03 16:17]: "; !strings.HasSuffix(h.tty.String(), want) {
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
		if want := "(in " + when.Delay(runAt-answered) + ")\n"; !strings.Contains(h.tty.String(), want) {
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
		for _, want := range []string{"2026-10-03 16:18 has already passed.\n", "  → Sat 2026-10-03 16:20:00 (in 2m)\n"} {
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
	if !strings.Contains(string(service), "ExecStart=/opt/schedule-send __fire "+id+"\n") {
		t.Errorf("service:\n%s", service)
	}
	if !strings.HasSuffix(h.stdout.String(), "payload=two\n\ncancel with: schedule-send --cancel "+id+"\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
}
