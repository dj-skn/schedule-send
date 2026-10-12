package job

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestConcurrentWriteReservesOneCompleteJob(t *testing.T) {
	p := Paths{StateDir: filepath.Join(t.TempDir(), "state")}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, msg := range []string{"first", "second"} {
		wg.Add(1)
		go func(msg string) {
			defer wg.Done()
			<-start
			results <- p.Write(State{ID: "same", Keys: msg, Grace: 4}, &msg)
		}(msg)
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

func TestValidatePreventsMalformedJobsFromFallingThrough(t *testing.T) {
	s := State{ID: "valid", Kind: "agent", PaneID: "wA:p1", Agent: "claude", Cwd: "/tmp", Mode: "text", IfBusy: "wait", Recover: "keys", Grace: 4}
	if err := s.Validate("valid"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*State){func(s *State) { s.Kind = "" }, func(s *State) { s.Mode = "garbage" }, func(s *State) { s.PaneID = "" }, func(s *State) { s.ID = "other" }, func(s *State) { s.Grace = 0 }, func(s *State) { s.Force = 2 }, func(s *State) { s.Quiet = 2 }, func(s *State) { s.RunAt = -1 }} {
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
