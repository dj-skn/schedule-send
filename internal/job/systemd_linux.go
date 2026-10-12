package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dj-skn/schedule-send/internal/installpath"
)

// This file is the systemd --user side of a job: the timer and service units
// it is armed with, and the order they are stopped, removed, and reloaded in.

// Scheduler is what arms, inspects, and removes a job's timer on this
// platform: systemctl --user. Each platform's file defines one with the same
// methods, and the build picks it.
type Scheduler = Systemctl

// Available says why jobs cannot be scheduled here, as the message shown to
// the user. Arming also needs /usr/bin/env, which every service runs through.
func (s Systemctl) Available(arming bool) error {
	if arming {
		fi, err := os.Stat("/usr/bin/env")
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
			return errors.New("/usr/bin/env is required to execute scheduled jobs (GNU coreutils).")
		}
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl is required.")
	}
	// is-system-running exits non-zero for "degraded", which merely means some
	// unrelated unit failed earlier. Only a manager we cannot talk to at all is
	// a real blocker, and that prints nothing.
	if s.Output("is-system-running") == "" {
		return errors.New("systemd --user is not available for this account.")
	}
	return nil
}

// Files names what arming writes for a job, as a receipt shows it.
func (Systemctl) Files(p Paths, id string) string {
	return p.UnitDir + "/" + UnitPrefix + "-" + id + ".{service,timer}"
}

// Trigger is a job's rendered units, checked before the job is reserved.
type Trigger struct{ id, service, timer string }

// Render builds a job's units without touching the disk. pane and calendar
// describe it, self is the installed executable it runs, and runAt is the
// instant it fires, which the timer carries in UTC.
func (Systemctl) Render(pane, calendar, self, id string, runAt int64, persistent bool) (Trigger, error) {
	service, err := ServiceUnit(pane, calendar, self, id)
	if err != nil {
		return Trigger{}, err
	}
	utc := time.Unix(runAt, 0).UTC().Format("2006-01-02 15:04:05") + " UTC"
	return Trigger{id: id, service: service, timer: TimerUnit(id, utc, persistent)}, nil
}

// Arm writes a job's units under p and enables its timer. attempted reports
// whether enabling was tried, which decides how a failure is rolled back. An
// error is the message shown to the user.
func (s Systemctl) Arm(p Paths, t Trigger) (attempted bool, err error) {
	// Unit files live on disk rather than being transient, because transient
	// units are written to /run and would be lost on reboot.
	if err := os.MkdirAll(p.UnitDir, 0o777); err != nil {
		return false, fmt.Errorf("Could not create %s: %v", p.UnitDir, err)
	}
	service, timer := p.UnitPath(t.id, "service"), p.UnitPath(t.id, "timer")
	if err := AtomicWrite(service, []byte(t.service), 0o644); err != nil {
		return false, fmt.Errorf("Could not write %s: %v", service, err)
	}
	if err := AtomicWrite(timer, []byte(t.timer), 0o644); err != nil {
		return false, fmt.Errorf("Could not write %s: %v", timer, err)
	}
	if err := s.Quiet("daemon-reload"); err != nil {
		return false, fmt.Errorf("systemctl daemon-reload failed: %v", err)
	}
	unit := UnitPrefix + "-" + t.id
	if err := s.Quiet("enable", "--now", unit+".timer"); err != nil {
		return true, fmt.Errorf("Could not arm %s.timer: %v", unit, err)
	}
	return true, nil
}

// defaultUnitDir is where systemd --user reads this account's units.
func defaultUnitDir(home, config string) string { return config + "/systemd/user" }

// UnitPath names one of a job's unit files.
func (p Paths) UnitPath(id, ext string) string {
	return filepath.Join(p.UnitDir, UnitPrefix+"-"+id+"."+ext)
}

// ServiceUnit is the oneshot service a job's timer starts.
func ServiceUnit(pane, calendar, self, id string) (string, error) {
	for _, value := range []string{pane, calendar, self, id} {
		if !utf8.ValidString(value) {
			return "", errors.New("service metadata must be UTF-8")
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return "", errors.New("service metadata contains a control character")
			}
		}
	}
	if err := installpath.Check(self); err != nil {
		return "", fmt.Errorf("invalid service executable: %w", err)
	}
	if !validID.MatchString(id) {
		return "", errors.New("invalid job id")
	}
	// systemd command syntax is not shell syntax: quote the executable and
	// escape C-style characters, specifiers, and variable references. env
	// executes the path as an argument, since systemd rejects quotes and
	// backslashes in its executable token even when escaped. "$$" is used
	// rather than the ':' prefix, which needs systemd 249 or newer.
	executable := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`).Replace(self)
	description := strings.ReplaceAll("herdr scheduled send to "+pane+" at "+calendar, "%", "%%")
	return fmt.Sprintf(`[Unit]
Description=%s

[Service]
Type=oneshot
ExecStart=/usr/bin/env -- "%s" __fire %s
`, description, executable, id), nil
}

// TimerUnit fires once at calendar, a resolved wall-clock time, so what
// systemd fires on is exactly what was printed.
func TimerUnit(id, calendar string, persistent bool) string {
	return fmt.Sprintf(`[Unit]
Description=herdr scheduled send timer for %s

[Timer]
OnCalendar=%s
AccuracySec=1s
RemainAfterElapse=no
Persistent=%t

[Install]
WantedBy=timers.target
`, id, calendar, persistent)
}

// Systemctl runs systemctl --user with a deadline. Output includes non-zero
// is-active responses; mutating callers receive the diagnostics as errors.
type Systemctl struct{}

func systemctlCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	return command(ctx, time.Second, "systemctl", append([]string{"--user"}, args...)...), cancel
}

// Capture runs systemctl --user under the caller's deadline, for inspection
// that needs the exit status and stderr separately.
func (Systemctl) Capture(ctx context.Context, drain time.Duration, args ...string) Result {
	return Run(ctx, drain, "systemctl", append([]string{"--user"}, args...)...)
}
func (Systemctl) Quiet(args ...string) error {
	cmd, cancel := systemctlCommand(args...)
	defer cancel()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
func (Systemctl) Output(args ...string) string {
	cmd, cancel := systemctlCommand(args...)
	defer cancel()
	out, _ := cmd.Output()
	return strings.TrimRight(string(out), "\n")
}
func (Systemctl) Through(stdout, stderr io.Writer, args ...string) error {
	cmd, cancel := systemctlCommand(args...)
	defer cancel()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

// Missing units are already stopped; transport errors are never treated as
// missing. Diagnostics are read only from a real systemctl exit failure.
func missingUnit(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "not loaded") || strings.Contains(text, "does not exist") || strings.Contains(text, "could not be found")
}
func stopUnit(sys Systemctl, args ...string) error {
	err := sys.Quiet(args...)
	if missingUnit(err) {
		return nil
	}
	return err
}
func (p Paths) disarm(sys Systemctl, id string, stopService bool) error {
	unit := UnitPrefix + "-" + id
	if err := stopUnit(sys, "stop", unit+".timer"); err != nil {
		return err
	}
	if stopService {
		if err := stopUnit(sys, "stop", unit+".service"); err != nil {
			return err
		}
	}
	return stopUnit(sys, "disable", unit+".timer")
}

func (p Paths) removeFiles(id string) error {
	for _, path := range []string{p.UnitPath(id, "timer"), p.UnitPath(id, "service"), p.MessagePath(id), p.StatePath(id)} {
		if err := removeFile(path); err != nil {
			return err
		}
	}
	return nil
}

// finishRemoval retains state/message until removed units have been reloaded.
// That leaves a failed cleanup inspectable and safe to retry.
func (p Paths) finishRemoval(sys Systemctl, id string) error {
	for _, path := range []string{p.UnitPath(id, "timer"), p.UnitPath(id, "service")} {
		if err := removeFile(path); err != nil {
			return err
		}
	}
	if err := sys.Quiet("daemon-reload"); err != nil {
		return err
	}
	if err := removeFile(p.MessagePath(id)); err != nil {
		return err
	}
	return removeFile(p.StatePath(id))
}

// Remove keeps inspectable state when stopping/disabling a job fails.
func (p Paths) Remove(sys Systemctl, id string) error {
	lock, err := p.Lock(id)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(p.StatePath(id)); err != nil {
		return err
	}
	if err := p.disarm(sys, id, true); err != nil {
		return err
	}
	_ = sys.Quiet("reset-failed", UnitPrefix+"-"+id+".timer", UnitPrefix+"-"+id+".service")
	return p.finishRemoval(sys, id)
}

// Rollback is used while the scheduler still holds the reservation. Before
// enable was attempted, none of the job's units can be running. Afterwards
// it must be stopped before deleting the state, even if enable returned an error.
func (p Paths) Rollback(sys Systemctl, id string, activationAttempted bool) error {
	if activationAttempted {
		if err := p.disarm(sys, id, true); err != nil {
			return err
		}
	}
	if err := p.removeFiles(id); err != nil {
		return err
	}
	// Failed reloads cannot arm a timer. Files have been removed and a later
	// successful reload will reconcile the manager's metadata.
	_ = sys.Quiet("daemon-reload")
	return nil
}

// Cleanup never stops its own service. Retain state on cleanup failures so
// --list and cancellation can repair the remaining artifacts.
func (p Paths) Cleanup(sys Systemctl, id string) error {
	lock, err := p.lockUntil(id, 35*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := p.disarm(sys, id, false); err != nil {
		return err
	}
	return p.finishRemoval(sys, id)
}

// Sweep only removes orphan units after acquiring the same lock as scheduling.
func (p Paths) Sweep(sys Systemctl) {
	_ = sys.Quiet("reset-failed", UnitPrefix+"-*")
	timers, _ := filepath.Glob(filepath.Join(p.UnitDir, UnitPrefix+"-*.timer"))
	swept := false
	for _, t := range timers {
		id := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(t), ".timer"), UnitPrefix+"-")
		lock, err := p.Lock(id)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p.StatePath(id)); errors.Is(err, os.ErrNotExist) {
			if err := p.disarm(sys, id, false); err == nil {
				if err := removeFile(p.UnitPath(id, "timer")); err == nil {
					_ = removeFile(p.UnitPath(id, "service"))
					swept = true
				}
			}
		}
		lock.Close()
	}
	if swept {
		_ = sys.Quiet("daemon-reload")
	}
}
