package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"schedule-send/internal/job"
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
	if !strings.Contains(h.stderr.String(), "Multiple targets matched.") || !strings.Contains(h.stderr.String(), "  5  · zsh      wB:p5") {
		t.Errorf("stderr:\n%s", h.stderr)
	}
}

func TestPickerAndConfirmOnTerminal(t *testing.T) {
	input := "2\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	drawn := h.tty.String()
	for _, want := range []string{
		"target [1]: ",
		"Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\nResets Sat 16:00 (in 38m); sending \"resume\" 1 min after.\n",
		"at [2026-10-03 16:01]: ",
	} {
		if !strings.Contains(drawn, want) {
			t.Errorf("terminal lacks %q:\n%s", want, drawn)
		}
	}
	if !strings.Contains(h.stdout.String(), "target=agent wB:p1 (claude, idle)\n") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
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
