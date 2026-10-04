// Package herdr runs the herdr CLI and decodes what it returns.
package herdr

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"schedule-send/internal/target"
)

// Locate finds the herdr binary: HERDR_BIN_OVERRIDE when set, otherwise herdr
// on PATH, as an absolute path with symlinks resolved.
func Locate() (string, error) {
	bin := os.Getenv("HERDR_BIN_OVERRIDE")
	if bin == "" {
		bin, _ = exec.LookPath("herdr")
	}
	if bin == "" || !executable(bin) {
		return "", errors.New("herdr is not on PATH.")
	}
	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}
	return bin, nil
}

func executable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// Client runs one herdr binary.
type Client struct {
	Bin string
	// Stderr receives herdr's own error output for the calls whose failures
	// the user should see. Nil discards it.
	Stderr io.Writer
}

func (c Client) cmd(stderr io.Writer, args ...string) *exec.Cmd {
	cmd := exec.Command(c.Bin, args...)
	cmd.Stderr = stderr
	return cmd
}

// output runs herdr and returns its stdout, also when it failed.
func (c Client) output(stderr io.Writer, args ...string) (string, error) {
	out, err := c.cmd(stderr, args...).Output()
	return string(out), err
}

type agentJSON struct {
	PaneID         string          `json:"pane_id"`
	Agent          *string         `json:"agent"`
	AgentStatus    *string         `json:"agent_status"`
	Cwd            *string         `json:"cwd"`
	ForegroundCwd  *string         `json:"foreground_cwd"`
	TitleStripped  *string         `json:"terminal_title_stripped"`
	Focused        bool            `json:"focused"`
	StateChangeSeq json.RawMessage `json:"state_change_seq"`
}

func or(s *string, def string) string {
	if s == nil || *s == "" {
		return def
	}
	return *s
}

func (a agentJSON) cwd() string { return or(a.Cwd, or(a.ForegroundCwd, "-")) }

func (c Client) agents() ([]agentJSON, error) {
	out, err := c.output(c.Stderr, "agent", "list")
	if err != nil {
		return nil, err
	}
	var v struct {
		Result struct {
			Agents []agentJSON `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return nil, err
	}
	return v.Result.Agents, nil
}

// Targets lists every agent, then every plain pane, as picker rows.
func (c Client) Targets() ([]target.Row, error) {
	agents, err := c.agents()
	if err != nil {
		return nil, err
	}
	var rows []target.Row
	for _, a := range agents {
		rows = append(rows, target.Row{
			Kind: "agent", Pane: a.PaneID, Agent: or(a.Agent, "agent"),
			Status: or(a.AgentStatus, "unknown"), Cwd: a.cwd(), Title: or(a.TitleStripped, "-"),
		})
	}

	out, err := c.output(c.Stderr, "pane", "list")
	if err != nil {
		return nil, err
	}
	var v struct {
		Result struct {
			Panes []agentJSON `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return nil, err
	}
	for _, p := range v.Result.Panes {
		if p.Agent != nil {
			continue
		}
		rows = append(rows, target.Row{
			Kind: "pane", Pane: p.PaneID, Agent: "-", Status: "-",
			Cwd: p.cwd(), Title: or(p.TitleStripped, "-"),
		})
	}
	return rows, nil
}

// ProcName is a plain pane's foreground process, for the picker label, or "-".
func (c Client) ProcName(pane string) string {
	out, err := c.output(nil, "pane", "process-info", "--pane", pane)
	if err != nil {
		return "-"
	}
	var v struct {
		Result struct {
			ProcessInfo struct {
				Foreground []struct {
					Name *string `json:"name"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(out), &v) != nil || len(v.Result.ProcessInfo.Foreground) == 0 {
		return "-"
	}
	return or(v.Result.ProcessInfo.Foreground[0].Name, "-")
}

// Read returns a pane's text from source ("visible" or "recent-unwrapped"),
// or whatever came back before a failure.
func (c Client) Read(pane, source string, lines int) string {
	out, _ := c.output(nil, "agent", "read", pane, "--source", source, "--lines", strconv.Itoa(lines))
	return out
}

// State is an agent's lifecycle status and its state_change_seq, a counter
// herdr stamps whenever the agent's own status transitions. Seq is -1 when
// herdr does not report it, and status "unknown" when the agent cannot be
// read at all.
func (c Client) State(pane string) (status string, seq int64) {
	out, err := c.output(nil, "agent", "get", pane)
	if err != nil {
		return "unknown", -1
	}
	var v struct {
		Result struct {
			Agent agentJSON `json:"agent"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(out), &v) != nil {
		return "unknown", -1
	}
	seq = -1
	if n, err := strconv.ParseInt(string(v.Result.Agent.StateChangeSeq), 10, 64); err == nil {
		seq = n
	}
	return or(v.Result.Agent.AgentStatus, "unknown"), seq
}

// Prompt submits text to an agent, returning herdr's combined output for the
// error it explains.
func (c Client) Prompt(pane, msg string) (string, error) {
	out, err := exec.Command(c.Bin, "agent", "prompt", pane, msg).CombinedOutput()
	return string(out), err
}

// PromptError is the message in herdr's JSON error reply, or its first line.
func PromptError(out string) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(out), &v) == nil && v.Error.Message != "" {
		return v.Error.Message
	}
	for i, ch := range out {
		if ch == '\n' {
			return out[:i]
		}
	}
	return out
}

// SendKeys presses keys in a pane. surface is "agent", which validates the
// agent's identity first, or "pane" for a plain terminal. stderr receives
// herdr's error output; nil discards it.
func (c Client) SendKeys(surface, pane string, keys []string, stderr io.Writer) error {
	return c.cmd(stderr, append([]string{surface, "send-keys", pane}, keys...)...).Run()
}

// Run types text into a plain pane and presses Enter.
func (c Client) Run(pane, msg string, stderr io.Writer) error {
	return c.cmd(stderr, "pane", "run", pane, msg).Run()
}

// Wait blocks until the agent settles or the timeout passes.
func (c Client) Wait(pane string, timeoutMs int) error {
	return c.cmd(nil, "agent", "wait", pane, "--timeout", strconv.Itoa(timeoutMs)).Run()
}

// Focus focuses an agent's pane.
func (c Client) Focus(pane string) error {
	return c.cmd(nil, "agent", "focus", pane).Run()
}

// Focused is the pane id of the focused agent, or "".
func (c Client) Focused() string {
	c.Stderr = nil
	agents, err := c.agents()
	if err != nil {
		return ""
	}
	for _, a := range agents {
		if a.Focused {
			return a.PaneID
		}
	}
	return ""
}

// Notify raises a Herdr notification, ignoring failure.
func (c Client) Notify(title, body string) {
	_ = c.cmd(nil, "notification", "show", title, "--body", body, "--sound", "request").Run()
}
