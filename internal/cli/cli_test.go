//go:build linux

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/when"
)

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

func TestListShowsCorruptStateWithDiagnostic(t *testing.T) {
	h := newHarness(t, nil)
	h.addJobs(t)
	os.WriteFile(h.a.paths.StatePath("bad-job"), []byte("corrupt"), 0600)
	if code := h.a.main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, h.stderr)
	}
	jobs := jsonObject(t, h.stdout.Bytes())["jobs"].([]any)
	if len(jobs) != 3 {
		t.Fatalf("stdout %s", h.stdout)
	}
	found := false
	for _, v := range jobs {
		row := v.(map[string]any)
		if row["id"] == "bad-job" {
			found = row["state"] == "invalid" && row["error"] != nil
		} else if _, ok := row["error"]; ok {
			t.Errorf("valid job has an error: %v", row)
		}
	}
	if !found {
		t.Fatalf("corrupt job not listed as invalid: %s", h.stdout)
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
	if !strings.Contains(h.stderr.String(), "or tools/install from a checkout, then run the installed schedule-send.") {
		t.Fatalf("stderr: %s", h.stderr)
	}
	for _, dir := range []string{h.a.paths.StateDir, h.a.paths.UnitDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("created job directory %s: %v", dir, err)
		}
	}
}

// A dry run previews the installed executable, not whichever build ran it, and
// says so when that build could not arm the job it previews.
func TestDryRunFromUnmanagedBuild(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		h := newHarness(t, nil)
		h.a.installedPath = func() (string, error) { return "", errors.New("this build has no installation path") }
		args := []string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello", "--dry-run"}
		if jsonMode {
			args = append(args, "--json")
		}
		if code := h.a.main(args); code != 0 {
			t.Fatalf("exit %d: %s", code, h.stderr)
		}
		if !strings.Contains(h.stderr.String(), "Warning: this build cannot arm jobs (this build has no installation path), so a real run would be refused. Install it again with Homebrew") {
			t.Errorf("stderr: %s", h.stderr)
		}
		if strings.Contains(h.stdout.String(), "__fire") {
			t.Errorf("previewed a command that would not be armed: %s", h.stdout)
		}
		if !jsonMode && !strings.Contains(h.stdout.String(), "would_run=\n") {
			t.Errorf("stdout: %s", h.stdout)
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

// keepGoCaches pins Go's caches before a harness points HOME at a directory
// that does not exist, so the builds a test runs do not look for them there.
func keepGoCaches(t *testing.T) {
	t.Helper()
	out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	dirs := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(dirs) != 2 {
		t.Fatalf("go env: unexpected output %q", out)
	}
	t.Setenv("GOCACHE", dirs[0])
	t.Setenv("GOMODCACHE", dirs[1])
}

// Exercise the real linked metadata and CLI with isolated Herdr/systemd stand-ins.
func TestBuiltInstallationAndUnmanagedCommands(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	keepGoCaches(t)
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
				if len(jobs) != 2 {
					t.Fatalf("jobs: %s", h.stdout)
				}
				for _, v := range jobs {
					row := v.(map[string]any)
					if row["id"] != id {
						continue
					}
					if row["state"] != "invalid" || !strings.Contains(row["error"].(string), "job field grace") || row["pane"] != "wB:p1" {
						t.Fatalf("malformed job row: %v", row)
					}
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
