// Package job is a scheduled send on disk: its state file, its message, and
// the systemd --user timer and service that fire it.
package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"schedule-send/internal/installpath"
)

// UnitPrefix names every unit this tool writes.
const UnitPrefix = "herdr-send"

// Paths are where jobs live.
type Paths struct {
	StateDir string // state and message files, mode 700
	UnitDir  string // systemd --user unit files
}

// DefaultPaths follows XDG, as the shell version did.
func DefaultPaths() Paths {
	home := os.Getenv("HOME")
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = home + "/.local/state"
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = home + "/.config"
	}
	return Paths{StateDir: state + "/herdr-send", UnitDir: config + "/systemd/user"}
}

// StatePath, MessagePath, and UnitPath name a job's files.
func (p Paths) StatePath(id string) string   { return filepath.Join(p.StateDir, id+".json") }
func (p Paths) MessagePath(id string) string { return filepath.Join(p.StateDir, id+".msg") }
func (p Paths) UnitPath(id, ext string) string {
	return filepath.Join(p.UnitDir, UnitPrefix+"-"+id+"."+ext)
}

// State is a job's state file. It is read by whatever version is installed
// when the timer fires, which may be older or newer than the one that wrote
// it, so keys are only ever added, with defaults for files that lack them.
type State struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	PaneID   string `json:"pane_id"`
	Agent    string `json:"agent"`
	Cwd      string `json:"cwd"`
	At       string `json:"at"`
	Mode     string `json:"mode"`
	Keys     string `json:"keys"`
	IfBusy   string `json:"if_busy"`
	Force    int    `json:"force"`
	Quiet    int    `json:"quiet"`
	Recover  string `json:"recover"`
	Grace    int    `json:"grace"`
	HerdrBin string `json:"herdr_bin"`
	Unit     string `json:"unit"`
	RunAt    int64  `json:"run_at,omitempty"`
}

// Load reads a state file, retaining defaults for absent/null optional fields.
// Numeric fields use JSON integer literals and are range-checked before use.
func Load(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return State{}, err
	}
	s := State{Grace: 4}
	for _, field := range []struct {
		name string
		dest *string
	}{
		{"id", &s.ID}, {"kind", &s.Kind}, {"pane_id", &s.PaneID},
		{"agent", &s.Agent}, {"cwd", &s.Cwd}, {"at", &s.At},
		{"mode", &s.Mode}, {"keys", &s.Keys}, {"if_busy", &s.IfBusy},
		{"recover", &s.Recover}, {"herdr_bin", &s.HerdrBin}, {"unit", &s.Unit},
	} {
		if value := raw[field.name]; len(value) != 0 {
			if err := json.Unmarshal(value, field.dest); err != nil {
				return State{}, fmt.Errorf("job field %s must be a string", field.name)
			}
		}
	}
	for _, field := range []struct {
		name          string
		dest          *int
		def, min, max int64
	}{
		{"force", &s.Force, 0, 0, 1},
		{"quiet", &s.Quiet, 0, 0, 1},
		{"grace", &s.Grace, 4, 1, 60},
	} {
		value, err := integer(raw[field.name], field.name, field.def, field.min, field.max)
		if err != nil {
			return State{}, err
		}
		*field.dest = int(value)
	}
	s.RunAt, err = integer(raw["run_at"], "run_at", 0, 0, 1<<63-1)
	if err != nil {
		return State{}, err
	}
	if s.IfBusy == "" {
		s.IfBusy = "wait"
	}
	if s.Recover == "" {
		s.Recover = "keys"
	}
	return s, nil
}

func integer(raw json.RawMessage, name string, def, min, max int64) (int64, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return def, nil
	}
	var value int64
	// Decoding directly into int64 rejects decimals/exponents and overflow,
	// and avoids both float64 precision loss and platform-dependent int sizes.
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("job field %s must be a JSON integer representable as int64", name)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("job field %s must be between %d and %d", name, min, max)
	}
	return value, nil
}

// Encode renders a state file as jq printed it: two-space indent, keys in
// their fixed order, no HTML escaping, and a final newline.
func Encode(s State) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
	return b.Bytes()
}

// ErrBusy means another process is changing this job.
var ErrBusy = errors.New("job is busy in another process")
var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Validate checks fields required to fire safely, while retaining defaults for
// old optional fields. A malformed kind must never fall through to pane run.
func (s State) Validate(id string) error {
	if !validID.MatchString(id) || s.ID != id {
		return errors.New("invalid or mismatched job id")
	}
	if s.Kind != "agent" && s.Kind != "pane" {
		return errors.New("invalid job kind")
	}
	if s.PaneID == "" || s.Cwd == "" || s.Kind == "agent" && s.Agent == "" {
		return errors.New("job target identity is incomplete")
	}
	if s.Mode != "text" && s.Mode != "keys" {
		return errors.New("invalid job mode")
	}
	if s.Mode == "keys" && len(strings.Fields(s.Keys)) == 0 {
		return errors.New("job has no keys")
	}
	if s.IfBusy != "wait" && s.IfBusy != "abort" {
		return errors.New("invalid busy policy")
	}
	if s.Recover != "keys" && s.Recover != "focus" && s.Recover != "none" {
		return errors.New("invalid recovery policy")
	}
	if s.Grace < 1 || s.Grace > 60 || s.Force < 0 || s.Force > 1 || s.Quiet < 0 || s.Quiet > 1 {
		return errors.New("invalid job controls")
	}
	if s.RunAt < 0 {
		return errors.New("invalid firing epoch")
	}
	return nil
}

// Lock serializes reservation, arming, cancellation, and cleanup. Lock files
// remain in place: removing one would let another process lock a different inode.
type Lock struct{ file *os.File }

func (p Paths) Lock(id string) (*Lock, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("invalid job id")
	}
	if err := os.MkdirAll(p.StateDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(p.StateDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(p.StateDir, "."+id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &Lock{f}, nil
}
func (l *Lock) Close() error {
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// lockUntil lets a firing worker wait for the scheduler to finish enabling its
// timer. Scheduling itself remains nonblocking and cannot overwrite a job.
func (p Paths) lockUntil(id string, timeout time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		lock, err := p.Lock(id)
		if !errors.Is(err, ErrBusy) || !time.Now().Before(deadline) {
			return lock, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// AtomicWrite publishes a complete file in one rename within its directory.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".schedule-send-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// Write reserves and publishes a job. The scheduler holds the reservation
// through arming and uses Publish directly instead.
func (p Paths) Write(s State, msg *string) error {
	lock, err := p.Lock(s.ID)
	if err != nil {
		return err
	}
	defer lock.Close()
	return p.Publish(s, msg)
}

// Publish requires the caller to hold the job's lock. The message is complete
// before the state appears, so a worker never sees a half-written payload.
func (p Paths) Publish(s State, msg *string) error {
	if _, err := os.Stat(p.StatePath(s.ID)); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if msg != nil {
		if err := AtomicWrite(p.MessagePath(s.ID), []byte(*msg), 0o600); err != nil {
			return err
		}
	}
	if err := AtomicWrite(p.StatePath(s.ID), Encode(s), 0o600); err != nil {
		if msg != nil {
			_ = os.Remove(p.MessagePath(s.ID))
		}
		return err
	}
	return nil
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

// command is how every systemd tool runs: under ctx, in the C locale so
// diagnostics can be matched, and waiting at most drain for pipes a child
// process still holds after the command exits.
func command(ctx context.Context, drain time.Duration, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = drain
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

func systemctlCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	return command(ctx, time.Second, "systemctl", append([]string{"--user"}, args...)...), cancel
}

// Result is one command's separate output streams and how it ended. ExitCode
// is nil when the command never ran or was killed.
type Result struct {
	Stdout, Stderr string
	Err            error
	ExitCode       *int
}

// Run runs a systemd tool such as journalctl under ctx, keeping stdout and
// stderr apart. A context deadline is reported as the error even when the
// killed command exited in some other way.
func Run(ctx context.Context, drain time.Duration, name string, args ...string) Result {
	cmd := command(ctx, drain, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	r := Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		code := cmd.ProcessState.ExitCode()
		r.ExitCode = &code
	}
	return r
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
func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil
	}
	return err
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

// StateFiles lists the stored jobs' state files in name order.
func (p Paths) StateFiles() []string {
	files, _ := filepath.Glob(filepath.Join(p.StateDir, "*.json"))
	return files
}

// Preview is the first line of the message beside a state file, within its
// first 60 bytes, as --list shows it.
func Preview(stateFile string) string {
	b, err := os.ReadFile(strings.TrimSuffix(stateFile, ".json") + ".msg")
	if err != nil {
		return ""
	}
	if len(b) > 60 {
		end := 60
		for end > 0 && !utf8.RuneStart(b[end]) {
			end--
		}
		b = b[:end]
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}
