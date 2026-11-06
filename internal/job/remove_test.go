package job

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveIfSameChecksTheSelectedStateUnderLock(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "scheduler.log")
	const stub = `#!/bin/sh
printf '%s\n' "$*" >> "$SCHEDULE_SEND_TEST_SCHEDULER_LOG"
if [ "$SCHEDULE_SEND_TEST_DISARM_ERROR" = 1 ]; then
	echo 'lost bus' >&2
	exit 1
fi
`
	for _, name := range []string{"systemctl", "launchctl"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(stub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("SCHEDULE_SEND_TEST_SCHEDULER_LOG", log)

	p := Paths{StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "units")}
	s := State{ID: "job-1", Mode: "text"}
	msg := "original"
	if err := p.Write(s, &msg); err != nil {
		t.Fatal(err)
	}
	selected, err := os.Open(p.StatePath(s.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Close()
	original, err := selected.Stat()
	if err != nil {
		t.Fatal(err)
	}

	lock, err := p.Lock(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveIfSame(Scheduler{}, s.ID, original); !errors.Is(err, ErrBusy) {
		t.Errorf("busy job: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.StatePath(s.ID)); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveIfSame(Scheduler{}, s.ID, original); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing job: %v", err)
	}

	msg = "replacement"
	if err := p.Write(s, &msg); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveIfSame(Scheduler{}, s.ID, original); !errors.Is(err, ErrStateChanged) {
		t.Errorf("replaced job: %v", err)
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scheduler was contacted for an unavailable selection: %v", err)
	}
	if b, err := os.ReadFile(p.MessagePath(s.ID)); err != nil || string(b) != msg {
		t.Fatalf("replacement payload: %q, %v", b, err)
	}

	current, err := os.Stat(p.StatePath(s.ID))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCHEDULE_SEND_TEST_DISARM_ERROR", "1")
	if err := p.RemoveIfSame(Scheduler{}, s.ID, current); err == nil || !strings.Contains(err.Error(), "lost bus") {
		t.Fatalf("failed disarm: %v", err)
	}
	if got, err := os.Stat(p.StatePath(s.ID)); err != nil || !os.SameFile(got, current) {
		t.Fatalf("state did not survive failed disarm: %v", err)
	}
	t.Setenv("SCHEDULE_SEND_TEST_DISARM_ERROR", "0")
	if err := p.RemoveIfSame(Scheduler{}, s.ID, current); err != nil {
		t.Fatalf("confirmed job: %v", err)
	}
	if _, err := os.Stat(p.StatePath(s.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed job state remains: %v", err)
	}
}
