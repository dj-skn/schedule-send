// Package herdr runs the herdr CLI and decodes what it returns.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	// Timeout overrides command deadlines, primarily for tests.
	Timeout time.Duration
	// Stderr receives herdr's own error output for the calls whose failures
	// the user should see. Nil discards it.
	Stderr io.Writer
}

const commandTimeout = 10 * time.Second

// execute also checks Herdr's JSON error envelope: Herdr can reject a command
// while exiting zero. All subprocesses have deadlines, including pipe draining.
func (c Client) execute(timeout time.Duration, stderr io.Writer, args ...string) (string, string, error) {
	if c.Timeout > 0 {
		timeout = c.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.WaitDelay = 500 * time.Millisecond
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	err := cmd.Run()
	if stderr != nil {
		_, _ = stderr.Write(diagnostic.Bytes())
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		// agent read stdout is raw terminal text, which may itself contain
		// JSON examples or logs. Herdr command errors use stderr.
		if len(args) < 2 || args[0] != "agent" || args[1] != "read" {
			err = responseError(out.String())
		}
		if err == nil {
			err = responseError(diagnostic.String())
		}
	}
	return out.String(), diagnostic.String(), err
}

func responseError(out string) error {
	decode := func(s string) error {
		var v struct {
			Error *struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(s), &v) != nil || v.Error == nil {
			return nil
		}
		if v.Error.Message != "" {
			return errors.New(v.Error.Message)
		}
		if v.Error.Code != "" {
			return errors.New(v.Error.Code)
		}
		return errors.New("herdr rejected the command")
	}
	if err := decode(out); err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if err := decode(line); err != nil {
			return err
		}
	}
	return nil
}

// output returns stdout even when Herdr rejected the command.
func (c Client) output(stderr io.Writer, args ...string) (string, error) {
	out, _, err := c.execute(commandTimeout, stderr, args...)
	return out, err
}

func (c Client) run(timeout time.Duration, stderr io.Writer, args ...string) error {
	_, _, err := c.execute(timeout, stderr, args...)
	return err
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
// and reports failures separately from a valid empty screen.
func (c Client) Read(pane, source string, lines int) (string, error) {
	return c.output(nil, "agent", "read", pane, "--source", source, "--lines", strconv.Itoa(lines))
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
	out, diagnostic, err := c.execute(commandTimeout, nil, "agent", "prompt", pane, msg)
	return out + diagnostic, err
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
	return c.run(commandTimeout, stderr, append([]string{surface, "send-keys", pane}, keys...)...)
}

// Run types text into a plain pane and presses Enter.
func (c Client) Run(pane, msg string, stderr io.Writer) error {
	return c.run(commandTimeout, stderr, "pane", "run", pane, msg)
}

// Wait blocks until the agent settles or the timeout passes.
func (c Client) Wait(pane string, timeoutMs int) error {
	return c.run(time.Duration(timeoutMs)*time.Millisecond+commandTimeout, nil, "agent", "wait", pane, "--timeout", strconv.Itoa(timeoutMs))
}

// Focus focuses an agent's pane.
func (c Client) Focus(pane string) error {
	return c.run(commandTimeout, nil, "agent", "focus", pane)
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
	_ = c.run(2*time.Second, nil, "notification", "show", title, "--body", body, "--sound", "request")
}
