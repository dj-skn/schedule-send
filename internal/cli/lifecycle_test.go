package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"schedule-send/internal/job"
)

func replaceSystemctl(t *testing.T, h *harness, fail string) {
	t.Helper()
	script := "#!/bin/sh\n[ \"$2\" = is-system-running ] && { echo running; exit 0; }\n[ \"$2\" = \"" + fail + "\" ] && { echo 'lost bus' >&2; exit 1; }\nexit 0\n"
	path := filepath.Join(h.dir, "path/systemctl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFailedArmingRollsBackAndAllowsRetry(t *testing.T) {
	for _, fail := range []string{"enable", "daemon-reload"} {
		t.Run(fail, func(t *testing.T) {
			h := newHarness(t, nil)
			replaceSystemctl(t, h, fail)
			args := []string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello", "--json"}
			if code := h.a.main(args); code != 1 {
				t.Fatalf("exit=%d", code)
			}
			const id = "20261003-163000-wBp3"
			for _, path := range []string{h.a.paths.StatePath(id), h.a.paths.MessagePath(id), h.a.paths.UnitPath(id, "service"), h.a.paths.UnitPath(id, "timer")} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("orphan %s: %v", path, err)
				}
			}
			dec := json.NewDecoder(bytes.NewReader(h.stderr.Bytes()))
			for dec.More() {
				var obj map[string]any
				if err := dec.Decode(&obj); err != nil {
					t.Fatalf("non-JSON diagnostics %q: %v", h.stderr.String(), err)
				}
			}
			if !strings.Contains(h.stderr.String(), "lost bus") {
				t.Fatalf("missing diagnostics %s", h.stderr.String())
			}
			replaceSystemctl(t, h, "")
			h.stdout.Reset()
			h.stderr.Reset()
			if code := h.a.main(args); code != 0 {
				t.Fatalf("retry=%d %s", code, h.stderr.String())
			}
		})
	}
}

func TestUnitWriteFailureRollsBackState(t *testing.T) {
	h := newHarness(t, nil)
	if err := os.WriteFile(h.a.paths.UnitDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if files := h.a.paths.StateFiles(); len(files) != 0 {
		t.Fatalf("orphan states %v", files)
	}
}

func TestArmedTimerKeepsExactEpochInUTC(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}); code != 0 {
		t.Fatal(h.stderr.String())
	}
	const id = "20261003-163000-wBp3"
	s, err := job.Load(h.a.paths.StatePath(id))
	if err != nil {
		t.Fatal(err)
	}
	if s.RunAt != 1791037800 {
		t.Fatalf("run_at=%d", s.RunAt)
	}
	timer, err := os.ReadFile(h.a.paths.UnitPath(id, "timer"))
	if err != nil {
		t.Fatal(err)
	}
	want := "OnCalendar=" + time.Unix(s.RunAt, 0).UTC().Format("2006-01-02 15:04:05") + " UTC\n"
	if !strings.Contains(string(timer), want) {
		t.Fatalf("timer %s lacks %q", timer, want)
	}
}

func TestCancelFailureReportsErrorAndKeepsState(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.a.main([]string{"--pane", "wB:p3", "--at", "16:30", "--message", "hello"}); code != 0 {
		t.Fatal(h.stderr.String())
	}
	replaceSystemctl(t, h, "stop")
	h.stdout.Reset()
	h.stderr.Reset()
	if code := h.a.main([]string{"--cancel", "20261003-163000-wBp3", "--json"}); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if strings.Contains(h.stdout.String(), "cancelled") {
		t.Fatalf("false receipt %s", h.stdout.String())
	}
	if _, err := os.Stat(h.a.paths.StatePath("20261003-163000-wBp3")); err != nil {
		t.Fatalf("lost state: %v", err)
	}
}
