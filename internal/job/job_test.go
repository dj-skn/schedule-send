package job

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const zshJob = "../../testdata/fixtures/jobs/20261003-160100-wBp1"

func TestLoadAndEncodeMatchTheShellVersion(t *testing.T) {
	// The state file the zsh version wrote for a real job on 2026-10-03.
	want, err := os.ReadFile(zshJob + ".json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(zshJob + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if s.PaneID != "wB:p1" || s.Mode != "text" || s.Recover != "keys" || s.Grace != 4 || s.Force != 0 {
		t.Errorf("loaded %+v", s)
	}
	if got := Encode(s); !bytes.Equal(got, want) {
		t.Errorf("Encode differs from jq's output:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoadDefaultsForOlderFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.json")
	old := `{"id":"x","kind":"agent","pane_id":"wA:p1","mode":"keys","keys":"esc","force":1,"quiet":1}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Recover != "keys" || s.Grace != 4 || s.Force != 1 || s.Quiet != 1 || s.Keys != "esc" {
		t.Errorf("loaded %+v", s)
	}
	for _, grace := range []string{`-1`, `4.5`, `null`} {
		os.WriteFile(path, []byte(`{"grace":`+grace+`}`), 0o644)
		if s, _ := Load(path); s.Grace != 4 {
			t.Errorf("grace %s loaded as %d, want the default", grace, s.Grace)
		}
	}
}

func TestUnitsMatchTheShellVersion(t *testing.T) {
	service, _ := os.ReadFile(filepath.Join(filepath.Dir(zshJob), "herdr-send-20261003-160100-wBp1.service"))
	timer, _ := os.ReadFile(filepath.Join(filepath.Dir(zshJob), "herdr-send-20261003-160100-wBp1.timer"))
	got := ServiceUnit("wB:p1", "2026-10-03 16:01:00", "/home/dev/src/schedule-send/schedule-send.zsh", "20261003-160100-wBp1")
	if got != string(service) {
		t.Errorf("service:\n%s\nwant:\n%s", got, service)
	}
	if got := TimerUnit("20261003-160100-wBp1", "2026-10-03 16:01:00", false); got != string(timer) {
		t.Errorf("timer:\n%s\nwant:\n%s", got, timer)
	}
}

func TestWriteAndPreview(t *testing.T) {
	p := Paths{StateDir: filepath.Join(t.TempDir(), "herdr-send")}
	msg := "first line of a fairly long message that goes on past sixty bytes\nsecond"
	if err := p.Write(State{ID: "j1", Mode: "text"}, &msg); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p.StateDir)
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("state dir mode %v", fi.Mode().Perm())
	}
	if got := Preview(p.StatePath("j1")); got != msg[:60] {
		t.Errorf("Preview = %q", got)
	}
	short := "two\nlines"
	p.Write(State{ID: "j2"}, &short)
	if got := Preview(p.StatePath("j2")); got != "two" {
		t.Errorf("Preview = %q", got)
	}
	if got := len(p.StateFiles()); got != 2 {
		t.Errorf("StateFiles = %d", got)
	}
}
