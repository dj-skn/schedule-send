package job

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestConcurrentWriteReservesOneCompleteJob(t *testing.T) {
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state")}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, msg := range []string{"first", "second"} {
		wg.Add(1)
		go func(msg string) { defer wg.Done(); <-start; results <- p.Write(State{ID: "same", Keys: msg}, &msg) }(msg)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrBusy) && !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("successful writers = %d, want 1", success)
	}
	state, err := Load(p.StatePath("same"))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := os.ReadFile(p.MessagePath("same"))
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != state.Keys {
		t.Fatalf("inconsistent state/message: %q vs %q", state.Keys, msg)
	}
}

func TestLockSurvivesReleasingAnotherHandle(t *testing.T) {
	p := Paths{StateDir: t.TempDir()}
	first, err := p.Lock("same")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lock("same"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock: %v", err)
	}
	first.Close()
	second, err := p.Lock("same")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := p.Lock("same"); !errors.Is(err, ErrBusy) {
		t.Fatalf("third lock: %v", err)
	}
}

func failingSystemctl(t *testing.T, action string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$2\" = \"" + action + "\" ] && { echo 'lost bus' >&2; exit 1; }\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestCancelPreservesStateOnStopFailure(t *testing.T) {
	failingSystemctl(t, "stop")
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state"), UnitDir: t.TempDir()}
	msg := "hello"
	if err := p.Write(State{ID: "cancel"}, &msg); err != nil {
		t.Fatal(err)
	}
	if err := p.Remove(Systemctl{}, "cancel"); err == nil || !strings.Contains(err.Error(), "lost bus") {
		t.Fatalf("Remove returned %v", err)
	}
	for _, path := range []string{p.StatePath("cancel"), p.MessagePath("cancel")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("lost inspectable artifact: %v", err)
		}
	}
}

func TestRollbackBeforeActivationRemovesFilesDespiteUnavailableManager(t *testing.T) {
	failingSystemctl(t, "daemon-reload")
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state"), UnitDir: t.TempDir()}
	msg := "hello"
	if err := p.Write(State{ID: "failed"}, &msg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.UnitPath("failed", "service"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.Rollback(Systemctl{}, "failed", false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.StatePath("failed"), p.MessagePath("failed"), p.UnitPath("failed", "service")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact remains %s: %v", path, err)
		}
	}
}

func TestValidatePreventsMalformedJobsFromFallingThrough(t *testing.T) {
	s := State{ID: "valid", Kind: "agent", PaneID: "wA:p1", Agent: "claude", Cwd: "/tmp", Mode: "text", IfBusy: "wait", Recover: "keys", Grace: 4}
	if err := s.Validate("valid"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*State){func(s *State) { s.Kind = "" }, func(s *State) { s.Mode = "garbage" }, func(s *State) { s.PaneID = "" }, func(s *State) { s.ID = "other" }, func(s *State) { s.Grace = 0 }} {
		bad := s
		mutate(&bad)
		if err := bad.Validate("valid"); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestLoadRejectsWrongFieldTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"kind":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted numeric kind")
	}
}

func TestPreviewKeepsUTF8Boundary(t *testing.T) {
	p := Paths{StateDir: t.TempDir()}
	msg := strings.Repeat("a", 59) + "ą rest"
	if err := p.Write(State{ID: "utf8"}, &msg); err != nil {
		t.Fatal(err)
	}
	preview := Preview(p.StatePath("utf8"))
	if !utf8.ValidString(preview) || preview != strings.Repeat("a", 59) {
		t.Fatalf("invalid preview %q", preview)
	}
}

func TestCleanupWaitsForSchedulerReservation(t *testing.T) {
	failingSystemctl(t, "")
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state"), UnitDir: t.TempDir()}
	msg := "hello"
	if err := p.Write(State{ID: "immediate"}, &msg); err != nil {
		t.Fatal(err)
	}
	lock, err := p.Lock("immediate")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Cleanup(Systemctl{}, "immediate") }()
	time.Sleep(50 * time.Millisecond)
	lock.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.StatePath("immediate")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("job remains %v", err)
	}
}

func TestCleanupReloadFailureRetainsInspectableState(t *testing.T) {
	failingSystemctl(t, "daemon-reload")
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state"), UnitDir: t.TempDir()}
	msg := "hello"
	if err := p.Write(State{ID: "reload"}, &msg); err != nil {
		t.Fatal(err)
	}
	if err := p.Cleanup(Systemctl{}, "reload"); err == nil {
		t.Fatal("cleanup succeeded")
	}
	if _, err := os.Stat(p.StatePath("reload")); err != nil {
		t.Fatalf("lost state %v", err)
	}
}
