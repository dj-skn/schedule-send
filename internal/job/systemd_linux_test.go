package job

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
