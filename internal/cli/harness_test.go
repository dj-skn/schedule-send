package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/when"
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

// newHarness runs the command against the stand-in herdr and the stand-ins for
// each system's scheduler tools, so no test reaches systemd or launchd, at
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
	os.Symlink(filepath.Join(root, "testdata/standin/launchctl"), filepath.Join(path, "launchctl"))

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

// useDemo swaps the stand-in's screens for testdata/demo, the scenario
// tools/media records, at 13:24 UTC on 2026-10-15, from pane wC:p1.
func (h *harness) useDemo(t *testing.T) {
	t.Helper()
	root, _ := filepath.Abs("../..")
	herdrDir := filepath.Join(h.dir, "herdr")
	os.RemoveAll(herdrDir)
	os.MkdirAll(herdrDir, 0o755)
	entries, _ := os.ReadDir(filepath.Join(root, "testdata/demo"))
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(root, "testdata/demo", e.Name()))
		os.WriteFile(filepath.Join(herdrDir, e.Name()), b, 0o644)
	}
	t.Setenv("HERDR_SCHEDULE_NOW_EPOCH", "1792070640")
	t.Setenv("TZ", "UTC")
	when.Loc = time.UTC
	h.a.selfPane = "wC:p1"
}

func terminal() bool { return true }

func jsonObject(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, b)
	}
	return m
}

// dryRunRows is what the rail's end lists under a dry run's first line, for a
// send with id from cwd, as the harness runs it.
func (h *harness) dryRunRows(id, cwd string) string {
	return "   cwd      " + cwd + "\n   if-busy  wait\n   recover  keys\n   grace    4s\n   persist  no\n" +
		"   id       " + id + "\n   units    " + h.a.sys.Files(h.a.paths, id, false) + "\n" +
		"   runs     /opt/schedule-send __fire " + id + "\n"
}
