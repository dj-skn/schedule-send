// Package job is a scheduled send on disk: its state file, its message, and
// the timer that fires it, which each platform's own file arms and removes.
package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// UnitPrefix names every unit this tool writes.
const UnitPrefix = "herdr-send"

// Paths are where jobs live.
type Paths struct {
	StateDir string // state and message files, mode 700
	UnitDir  string // the scheduler's own files: systemd --user units on Linux
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
	return Paths{StateDir: state + "/herdr-send", UnitDir: defaultUnitDir(home, config)}
}

// StatePath and MessagePath name a job's files.
func (p Paths) StatePath(id string) string   { return filepath.Join(p.StateDir, id+".json") }
func (p Paths) MessagePath(id string) string { return filepath.Join(p.StateDir, id+".msg") }

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

// numericFields are the integer controls, their legacy defaults, and their
// bounds. Load and Validate both read this one table.
var numericFields = []struct {
	name          string
	def, min, max int64
	get           func(State) int64
	set           func(*State, int64)
}{
	{"force", 0, 0, 1, func(s State) int64 { return int64(s.Force) }, func(s *State, v int64) { s.Force = int(v) }},
	{"quiet", 0, 0, 1, func(s State) int64 { return int64(s.Quiet) }, func(s *State, v int64) { s.Quiet = int(v) }},
	{"grace", 4, 1, 60, func(s State) int64 { return int64(s.Grace) }, func(s *State, v int64) { s.Grace = int(v) }},
	{"run_at", 0, 0, 1<<63 - 1, func(s State) int64 { return s.RunAt }, func(s *State, v int64) { s.RunAt = v }},
}

// Load reads a state file, retaining defaults for absent/null optional fields.
// Numeric fields use JSON integer literals and are range-checked before use.
// On a field error Load still returns every field it could decode, with
// defaults for the rest, so a refused job can be reported and listed; such a
// state must never be delivered.
func Load(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return State{}, err
	}
	var s State
	var first error
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
			if err := json.Unmarshal(value, field.dest); err != nil && first == nil {
				first = fmt.Errorf("job field %s must be a string", field.name)
			}
		}
	}
	for _, field := range numericFields {
		value, err := integer(raw[field.name], field.name, field.def, field.min, field.max)
		if err != nil {
			if first == nil {
				first = err
			}
			value = field.def
		}
		field.set(&s, value)
	}
	if s.IfBusy == "" {
		s.IfBusy = "wait"
	}
	if s.Recover == "" {
		s.Recover = "keys"
	}
	return s, first
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
	for _, field := range numericFields {
		if v := field.get(s); v < field.min || v > field.max {
			return fmt.Errorf("job field %s must be between %d and %d", field.name, field.min, field.max)
		}
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

// command is how every scheduler tool runs: under ctx, in the C locale so
// diagnostics can be matched, and waiting at most drain for pipes a child
// process still holds after the command exits.
func command(ctx context.Context, drain time.Duration, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = drain
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

// Result is one command's separate output streams and how it ended. ExitCode
// is nil when the command never ran or was killed.
type Result struct {
	Stdout, Stderr string
	Err            error
	ExitCode       *int
}

// Run runs a scheduler tool such as journalctl under ctx, keeping stdout and
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

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil
	}
	return err
}

// Diagnostic is one finding from inspecting a job's timer, as --status
// reports it.
type Diagnostic struct {
	Source   string `json:"source"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Output   string `json:"output,omitempty"`
}

// Inspection is what --status shows of a job's timer and of its log: each as
// printed, and trimmed for JSON, with whatever went wrong collecting them.
type Inspection struct {
	TimerOutput, Timer     string
	JournalOutput, Journal string
	Diagnostics            []Diagnostic
}

func commandDiagnostic(source string, result Result) Diagnostic {
	code := "command_failed"
	switch {
	case errors.Is(result.Err, context.DeadlineExceeded):
		code = "timeout"
	case errors.Is(result.Err, exec.ErrWaitDelay):
		code = "pipe_timeout"
	case errors.Is(result.Err, exec.ErrNotFound), errors.Is(result.Err, os.ErrNotExist):
		code = "not_found"
	}
	message := result.Err.Error()
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		message += ": " + stderr
	}
	return Diagnostic{Source: source, Severity: "error", Code: code, Message: message, ExitCode: result.ExitCode, Stderr: result.Stderr, Output: result.Stdout}
}

func commandWarning(source string, result Result) Diagnostic {
	return Diagnostic{Source: source, Severity: "warning", Code: "command_stderr", Message: strings.TrimSpace(result.Stderr), ExitCode: result.ExitCode, Stderr: result.Stderr}
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
