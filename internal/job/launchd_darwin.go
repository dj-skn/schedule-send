package job

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dj-skn/schedule-send/internal/installpath"
	"github.com/dj-skn/schedule-send/internal/when"
)

// This file is the launchd side of a job: the agent it is armed with, in the
// user's login session, and the order it is unloaded and removed in.
//
// launchd differs from systemd in ways measured on macOS 26 and recorded in
// docs/ss-3-open-source-plan.md. Its calendar has no seconds and no year, a
// job is its own timer and service, booting a job out kills it at once, and a
// job starts with a minimal PATH.

// Scheduler is what arms, inspects, and removes a job's timer on this
// platform: launchctl, in the gui domain of the calling user.
type Scheduler struct{}

// soon is how close a firing time may be before the calendar is not trusted
// to catch it. Such a job is started when it is loaded, and waits.
const soon = 2 * time.Minute

// zone is the system's zone, which launchd's calendar is read in whatever TZ
// the scheduling shell has set.
var zone = func() *time.Location {
	if b, err := os.ReadFile("/etc/localtime"); err == nil {
		if loc, err := time.LoadLocationFromTZData("system", b); err == nil {
			return loc
		}
	}
	return time.Local
}

// defaultUnitDir is where launchd reads this account's agents at login.
func defaultUnitDir(home, config string) string { return home + "/Library/LaunchAgents" }

func domain() string          { return "gui/" + strconv.Itoa(os.Getuid()) }
func label(id string) string  { return UnitPrefix + "-" + id }
func target(id string) string { return domain() + "/" + label(id) }

// agentPath is a job's agent file. An ordinary job keeps it beside its state:
// launchd loads it from there, macOS records no background item for it, and
// launchd forgets it at logout, which is when such a job is skipped. A
// persistent job's file goes where launchd loads agents at login, the only
// place that survives a restart, and macOS announces each one.
func (p Paths) agentPath(id string, persistent bool) string {
	if persistent {
		return filepath.Join(p.UnitDir, label(id)+".plist")
	}
	return filepath.Join(p.StateDir, id+".plist")
}

// LogPath is where launchd writes a job's output. The log outlives the job so
// --status can still show it.
func (p Paths) LogPath(id string) string { return filepath.Join(p.StateDir, id+".log") }

func (Scheduler) run(args ...string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return Run(ctx, time.Second, "launchctl", args...)
}

// notLoaded is launchctl's answer for a job launchd does not have: 3 from
// bootout, 113 from print.
func notLoaded(r Result) bool {
	return r.ExitCode != nil && (*r.ExitCode == 3 || *r.ExitCode == 113)
}

// bootout unloads a job. One that is not loaded is already stopped; any
// other failure is reported, never taken for that.
func (s Scheduler) bootout(id string) error {
	r := s.run("bootout", target(id))
	if r.Err == nil || notLoaded(r) {
		return nil
	}
	return fmt.Errorf("launchctl bootout %s: %w: %s", label(id), r.Err, strings.TrimSpace(r.Stderr))
}

// Available says why jobs cannot be scheduled here, as the message shown to
// the user.
func (s Scheduler) Available(arming bool) error {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return errors.New("launchctl is required.")
	}
	if r := s.run("print", domain()); r.Err != nil {
		return fmt.Errorf("launchd has no login session for this account (%s).", domain())
	}
	if _, err := exec.LookPath(when.DateTool); err != nil {
		return fmt.Errorf("GNU date is required as %s. Install it with: brew install coreutils", when.DateTool)
	}
	return nil
}

// Name is a job's agent as receipts show it.
func (Scheduler) Name(id string) string { return label(id) }

// Files names what arming writes for a job, as a receipt shows it.
func (Scheduler) Files(p Paths, id string, persistent bool) string {
	return p.agentPath(id, persistent)
}

// Trigger is a job's agent, checked before the job is reserved.
type Trigger struct {
	id, self   string
	runAt      int64
	persistent bool
}

// Render checks what a job's agent is built from without touching the disk.
// self is the installed executable it runs and runAt the instant it fires.
// pane and calendar describe a systemd unit; an agent has no description.
func (Scheduler) Render(pane, calendar, self, id string, runAt int64, persistent bool) (Trigger, error) {
	if !utf8.ValidString(self) {
		return Trigger{}, errors.New("agent metadata must be UTF-8")
	}
	for _, r := range self {
		if unicode.IsControl(r) {
			return Trigger{}, errors.New("agent metadata contains a control character")
		}
	}
	if err := installpath.Check(self); err != nil {
		return Trigger{}, fmt.Errorf("invalid agent executable: %w", err)
	}
	if !validID.MatchString(id) {
		return Trigger{}, errors.New("invalid job id")
	}
	return Trigger{id: id, self: self, runAt: runAt, persistent: persistent}, nil
}

// agent renders a job's property list. A job due soon has no calendar and
// runs when it is loaded; a persistent one runs when loaded as well as on its
// calendar, so that a login after its time still starts it. The scheduling
// shell's PATH and XDG roots go with it, since launchd gives a job neither.
func agent(p Paths, t Trigger, atLoad bool) []byte {
	var b bytes.Buffer
	text := func(s string) string {
		var e bytes.Buffer
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	str := func(key, value string) {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", key, text(value))
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	str("Label", label(t.id))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range []string{t.self, "__fire", t.id} {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", text(arg))
	}
	b.WriteString("\t</array>\n")
	if !atLoad {
		at := time.Unix(t.runAt, 0).In(zone())
		b.WriteString("\t<key>StartCalendarInterval</key>\n\t<dict>\n")
		for _, field := range []struct {
			key   string
			value int
		}{{"Month", int(at.Month())}, {"Day", at.Day()}, {"Hour", at.Hour()}, {"Minute", at.Minute()}} {
			fmt.Fprintf(&b, "\t\t<key>%s</key>\n\t\t<integer>%d</integer>\n", field.key, field.value)
		}
		b.WriteString("\t</dict>\n")
	}
	if atLoad || t.persistent {
		b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	}
	str("StandardOutPath", p.LogPath(t.id))
	str("StandardErrorPath", p.LogPath(t.id))
	b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
	env := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", key, text(value))
		}
	}
	for _, key := range []string{"PATH", "XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		env(key, os.Getenv(key))
	}
	b.WriteString("\t</dict>\n</dict>\n</plist>\n")
	return b.Bytes()
}

// Arm writes a job's agent under p and loads it. attempted reports whether
// loading was tried, which decides how a failure is rolled back. An error is
// the message shown to the user.
func (s Scheduler) Arm(p Paths, t Trigger) (attempted bool, err error) {
	path, dir, mode := p.agentPath(t.id, t.persistent), p.StateDir, os.FileMode(0o600)
	if t.persistent {
		dir, mode = p.UnitDir, 0o644
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("Could not create %s: %v", dir, err)
	}
	atLoad := time.Until(time.Unix(t.runAt, 0)) < soon
	if err := AtomicWrite(path, agent(p, t, atLoad), mode); err != nil {
		return false, fmt.Errorf("Could not write %s: %v", path, err)
	}
	if r := s.run("bootstrap", domain(), path); r.Err != nil {
		return true, fmt.Errorf("Could not arm %s: launchctl bootstrap: %v: %s", label(t.id), r.Err, strings.TrimSpace(r.Stderr))
	}
	return true, nil
}

// Await holds a started worker until its job's firing time, and says whether
// this start is one to deliver on.
func (Scheduler) Await(runAt int64, now func() time.Time, sleep func(time.Duration)) (Verdict, string) {
	return await(runAt, now, sleep)
}

// Armed is a job's state as --list shows it: active while launchd has it
// loaded, "" otherwise.
func (s Scheduler) Armed(id string) string {
	if s.run("print", target(id)).Err == nil {
		return "active"
	}
	return ""
}

// Inspect collects what launchd says of a job and the end of its log. A job
// that is not loaded is an observation; a failed collection is a diagnostic.
func (Scheduler) Inspect(p Paths, id string, timerBudget, journalBudget, drain time.Duration) Inspection {
	var r Inspection
	ctx, cancel := context.WithTimeout(context.Background(), timerBudget)
	job := Run(ctx, drain, "launchctl", "print", target(id))
	expired := ctx.Err() != nil
	cancel()
	r.TimerOutput = job.Stdout
	r.Timer = strings.TrimRight(job.Stdout+job.Stderr, "\n")
	switch {
	case job.Err == nil:
		if job.Stderr != "" {
			r.Diagnostics = append(r.Diagnostics, commandWarning("timer", job))
		}
	case notLoaded(job) && !expired:
		r.Timer = label(id) + " is not loaded: it has run, was cancelled, or was never armed."
		r.TimerOutput = r.Timer + "\n"
	default:
		r.Diagnostics = append(r.Diagnostics, commandDiagnostic("timer", job))
	}

	b, err := os.ReadFile(p.LogPath(id))
	switch {
	case err == nil:
		lines := strings.SplitAfter(string(b), "\n")
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		r.JournalOutput = strings.Join(lines, "")
		r.Journal = strings.TrimRight(r.JournalOutput, "\n")
	case !errors.Is(err, os.ErrNotExist):
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Source: "journal", Severity: "error", Code: "read_failed", Message: fmt.Sprintf("Could not read the log for %s: %v", id, err)})
	}
	return r
}

// removeFiles removes the agent first, wherever it was written, and the state
// last, so a job that could not be fully removed is still listed and can be
// cancelled again.
func (p Paths) removeFiles(id string) error {
	for _, path := range []string{p.agentPath(id, false), p.agentPath(id, true), p.MessagePath(id), p.StatePath(id)} {
		if err := removeFile(path); err != nil {
			return err
		}
	}
	return nil
}

// Remove keeps inspectable state when unloading a job fails.
func (p Paths) Remove(s Scheduler, id string) error {
	lock, err := p.Lock(id)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(p.StatePath(id)); err != nil {
		return err
	}
	if err := s.bootout(id); err != nil {
		return err
	}
	return p.removeFiles(id)
}

// Rollback is used while the scheduler still holds the reservation. Before
// loading was attempted the job cannot be running. Afterwards it must be
// unloaded before its state is deleted, even if loading returned an error.
func (p Paths) Rollback(s Scheduler, id string, activationAttempted bool) error {
	if activationAttempted {
		if err := s.bootout(id); err != nil {
			return err
		}
	}
	return p.removeFiles(id)
}

// Cleanup runs inside the job it removes. Booting a job out kills it at
// once, so that comes last: by then its files and state are gone and nothing
// is left to do. State is retained when a file cannot be removed, so --list
// and cancellation can repair what remains.
func (p Paths) Cleanup(s Scheduler, id string) error {
	lock, err := p.lockUntil(id, 35*time.Second)
	if err != nil {
		return err
	}
	if err := p.removeFiles(id); err != nil {
		lock.Close()
		return err
	}
	lock.Close()
	return s.bootout(id)
}

// Sweep unloads and removes agents left without state, after acquiring the
// same lock as scheduling, and drops logs a week after their jobs are gone.
func (p Paths) Sweep(s Scheduler) {
	agents, _ := filepath.Glob(filepath.Join(p.UnitDir, UnitPrefix+"-*.plist"))
	beside, _ := filepath.Glob(filepath.Join(p.StateDir, "*.plist"))
	for _, a := range append(agents, beside...) {
		id := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(a), ".plist"), UnitPrefix+"-")
		lock, err := p.Lock(id)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p.StatePath(id)); errors.Is(err, os.ErrNotExist) {
			if s.bootout(id) == nil {
				_ = removeFile(a)
			}
		}
		lock.Close()
	}
	logs, _ := filepath.Glob(filepath.Join(p.StateDir, "*.log"))
	for _, l := range logs {
		id := strings.TrimSuffix(filepath.Base(l), ".log")
		if _, err := os.Stat(p.StatePath(id)); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		if info, err := os.Stat(l); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
			_ = removeFile(l)
		}
	}
}
