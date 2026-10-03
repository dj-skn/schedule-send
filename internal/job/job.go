// Package job is a scheduled send on disk: its state file, its message, and
// the systemd --user timer and service that fire it.
package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
}

// Load reads a state file. recover and grace default to "keys" and 4, so a
// timer armed before they existed still fires.
func Load(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return State{}, err
	}
	s := State{
		ID: str(raw["id"]), Kind: str(raw["kind"]), PaneID: str(raw["pane_id"]),
		Agent: str(raw["agent"]), Cwd: str(raw["cwd"]), At: str(raw["at"]),
		Mode: str(raw["mode"]), Keys: str(raw["keys"]), IfBusy: str(raw["if_busy"]),
		Force: whole(raw["force"], 0), Quiet: whole(raw["quiet"], 0),
		Recover: str(raw["recover"]), Grace: whole(raw["grace"], 4),
		HerdrBin: str(raw["herdr_bin"]), Unit: str(raw["unit"]),
	}
	if s.Recover == "" {
		s.Recover = "keys"
	}
	return s, nil
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func whole(v any, def int) int {
	f, ok := v.(float64)
	if !ok || f < 0 || f != float64(int(f)) {
		return def
	}
	return int(f)
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

// Write stores a job's state, and its message unless msg is nil. The state
// directory is created mode 700, since messages are stored verbatim.
func (p Paths) Write(s State, msg *string) error {
	if err := os.MkdirAll(p.StateDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(p.StateDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(p.StatePath(s.ID), Encode(s), 0o666); err != nil {
		return err
	}
	if msg != nil {
		return os.WriteFile(p.MessagePath(s.ID), []byte(*msg), 0o666)
	}
	return nil
}

// ServiceUnit is the oneshot service a job's timer starts.
func ServiceUnit(pane, calendar, self, id string) string {
	return fmt.Sprintf(`[Unit]
Description=herdr scheduled send to %s at %s

[Service]
Type=oneshot
ExecStart=%s __fire %s
`, pane, calendar, self, id)
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

// Systemctl runs systemctl --user.
type Systemctl struct{}

// Quiet runs it with all output discarded.
func (Systemctl) Quiet(args ...string) error {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).Run()
}

// Output runs it and returns its stdout, whatever the exit status: is-active
// prints "inactive" while exiting non-zero.
func (Systemctl) Output(args ...string) string {
	out, _ := exec.Command("systemctl", append([]string{"--user"}, args...)...).Output()
	return strings.TrimRight(string(out), "\n")
}

// Through runs it with its output passed to ours.
func (Systemctl) Through(stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

// Remove disarms a job and deletes its units and state, ignoring what is
// already gone.
func (p Paths) Remove(sys Systemctl, id string) {
	unit := UnitPrefix + "-" + id
	_ = sys.Quiet("disable", "--now", unit+".timer")
	_ = sys.Quiet("stop", unit+".service")
	_ = sys.Quiet("reset-failed", unit+".timer", unit+".service")
	os.Remove(p.UnitPath(id, "timer"))
	os.Remove(p.UnitPath(id, "service"))
	_ = sys.Quiet("daemon-reload")
	os.Remove(p.StatePath(id))
	os.Remove(p.MessagePath(id))
}

// Cleanup is what a fired job does to itself. A one-shot timer must not
// linger in timers.target.wants, or it would be re-armed at the next boot.
func (p Paths) Cleanup(sys Systemctl, id string) {
	os.Remove(p.StatePath(id))
	os.Remove(p.MessagePath(id))
	_ = sys.Quiet("disable", UnitPrefix+"-"+id+".timer")
	os.Remove(p.UnitPath(id, "timer"))
	os.Remove(p.UnitPath(id, "service"))
	_ = sys.Quiet("daemon-reload")
}

// Sweep clears failed units, which leave the user manager degraded, and drops
// unit files whose job finished or was never recorded.
func (p Paths) Sweep(sys Systemctl) {
	_ = sys.Quiet("reset-failed", UnitPrefix+"-*")
	timers, _ := filepath.Glob(filepath.Join(p.UnitDir, UnitPrefix+"-*.timer"))
	swept := false
	for _, t := range timers {
		id := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(t), ".timer"), UnitPrefix+"-")
		if _, err := os.Stat(p.StatePath(id)); err == nil {
			continue
		}
		_ = sys.Quiet("disable", UnitPrefix+"-"+id+".timer")
		os.Remove(p.UnitPath(id, "timer"))
		os.Remove(p.UnitPath(id, "service"))
		swept = true
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
		b = b[:60]
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}
