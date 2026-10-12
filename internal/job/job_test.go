package job

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	for _, grace := range []string{`null`} {
		os.WriteFile(path, []byte(`{"grace":`+grace+`}`), 0o644)
		if s, _ := Load(path); s.Grace != 4 {
			t.Errorf("grace %s loaded as %d, want the default", grace, s.Grace)
		}
	}
}

func TestUnitsMatchTheShellVersion(t *testing.T) {
	service, _ := os.ReadFile(filepath.Join(filepath.Dir(zshJob), "herdr-send-20261003-160100-wBp1.service"))
	timer, _ := os.ReadFile(filepath.Join(filepath.Dir(zshJob), "herdr-send-20261003-160100-wBp1.timer"))
	got, err := ServiceUnit("wB:p1", "2026-10-03 16:01:00", "/home/dev/src/schedule-send/schedule-send.zsh", "20261003-160100-wBp1")
	if err != nil {
		t.Fatal(err)
	}
	// New services quote the executable; legacy fixtures remain unchanged.
	service = bytes.Replace(service, []byte("ExecStart=/home/dev/src/schedule-send/schedule-send.zsh"), []byte(`ExecStart=/usr/bin/env -- "/home/dev/src/schedule-send/schedule-send.zsh"`), 1)
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

func TestServiceCommandEncoding(t *testing.T) {
	for _, tc := range []struct{ path, encoded string }{
		{"/opt/space name", `/opt/space name`},
		{`/opt/quote"name`, `/opt/quote\"name`},
		{`/opt/back\name`, `/opt/back\\name`},
		{"/opt/percent%name", `/opt/percent%%name`},
		{"/opt/dollar${HOME}", `/opt/dollar$${HOME}`},
		{"/opt/$HOME", `/opt/$$HOME`},
		{"/opt/two$$dollars", `/opt/two$$$$dollars`},
		{"/opt/apostrophe'name", `/opt/apostrophe'name`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			unit, err := ServiceUnit("pane%p", "2026-10-03 16:30:00", tc.path, "job-1")
			if err != nil {
				t.Fatal(err)
			}
			want := `ExecStart=/usr/bin/env -- "` + tc.encoded + `" __fire job-1` + "\n"
			if !strings.Contains(unit, want) {
				t.Fatalf("unit %q does not contain %q", unit, want)
			}
			if !strings.Contains(unit, "Description=herdr scheduled send to pane%%p") {
				t.Fatal("description specifier not escaped")
			}
		})
	}
	for _, args := range [][4]string{
		{"bad\npane", "date", "/opt/bin", "job"},
		{"pane", "bad\rdate", "/opt/bin", "job"},
		{"pane", "date", "/opt/bad\tbin", "job"},
		{"pane", "date", "/opt/bad\xff", "job"},
		{"pane", "date", "relative", "job"},
		{"pane", "date", "/opt/a=b/bin", "job"},
		{"pane", "date", "/opt//bin", "job"},
		{"pane", "date", "/opt/bin", "bad\njob"},
	} {
		if _, err := ServiceUnit(args[0], args[1], args[2], args[3]); err == nil {
			t.Errorf("accepted invalid metadata %q", args)
		}
	}
}
