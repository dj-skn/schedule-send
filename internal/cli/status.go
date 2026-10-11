package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"schedule-send/internal/job"
)

type statusTimeouts struct{ timer, journal, drain time.Duration }

type statusDiagnostic struct {
	Source   string `json:"source"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Output   string `json:"output,omitempty"`
}

type statusResult struct {
	State         json.RawMessage    `json:"state"`
	Timer         string             `json:"timer"`
	Journal       string             `json:"journal"`
	Warning       string             `json:"warning,omitempty"`
	Diagnostics   []statusDiagnostic `json:"diagnostics,omitempty"`
	rawState      []byte
	timerOutput   string
	journalOutput string
}

type statusCommand struct {
	stdout, stderr string
	err            error
	exitCode       *int
}

// Both modes use the same bounded collector. A failed unit is an observation;
// a failed collection returns an error after the partial result is rendered.
func (a *app) status(id string) error {
	result := a.collectStatus(id)
	if a.o.json {
		emit(a.stdout, result)
	} else {
		if len(result.rawState) > 0 {
			var pretty bytes.Buffer
			if json.Indent(&pretty, bytes.TrimSpace(result.rawState), "", "  ") == nil {
				pretty.WriteByte('\n')
				_, _ = a.stdout.Write(pretty.Bytes())
			} else {
				_, _ = a.stdout.Write(result.rawState)
			}
		} else if result.Warning != "" {
			fmt.Fprintln(a.stderr, result.Warning)
		}
		fmt.Fprintln(a.stdout)
		fmt.Fprint(a.stdout, result.timerOutput)
		fmt.Fprintln(a.stdout)
		fmt.Fprint(a.stdout, result.journalOutput)
		for _, d := range result.Diagnostics {
			fmt.Fprintf(a.stderr, "%s: %s\n", d.Source, d.Message)
		}
	}
	var failed []string
	for _, d := range result.Diagnostics {
		if d.Severity == "error" {
			failed = append(failed, d.Source)
		}
	}
	if len(failed) > 0 {
		return die("Status inspection incomplete: %s.", strings.Join(failed, ", "))
	}
	return nil
}

func (a *app) collectStatus(id string) statusResult {
	r := statusResult{State: json.RawMessage("null")}
	statePath := a.paths.StatePath(id)
	info, err := os.Stat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		r.Warning = fmt.Sprintf("No state file for %s (job may have already run).", id)
	} else {
		if err == nil && !info.Mode().IsRegular() {
			err = errors.New("state path is not a regular file")
		}
		if err == nil {
			r.rawState, err = os.ReadFile(statePath)
		}
		if err != nil {
			r.Diagnostics = append(r.Diagnostics, statusDiagnostic{Source: "state", Severity: "error", Code: "read_failed", Message: fmt.Sprintf("Could not read state file for %s: %v", id, err)})
		} else if !json.Valid(r.rawState) {
			r.Warning = fmt.Sprintf("Invalid JSON in state file for %s.", id)
			r.Diagnostics = append(r.Diagnostics, statusDiagnostic{Source: "state", Severity: "error", Code: "invalid_json", Message: r.Warning})
		} else {
			r.State = json.RawMessage(bytes.TrimSpace(r.rawState))
		}
	}
	limits := a.statusTimeouts
	if limits.timer <= 0 {
		limits.timer = 30 * time.Second
	}
	if limits.journal <= 0 {
		limits.journal = 10 * time.Second
	}
	if limits.drain <= 0 {
		limits.drain = time.Second
	}
	unit := job.UnitPrefix + "-" + id + ".timer"
	timerCtx, cancel := context.WithTimeout(context.Background(), limits.timer)
	timer := runStatusCommand(timerCtx, limits.drain, "systemctl", "--user", "status", unit, "--no-pager")
	r.timerOutput = timer.stdout
	r.Timer = strings.TrimRight(timer.stdout+timer.stderr, "\n")
	if timer.err != nil {
		normal := false
		if timer.exitCode != nil && (*timer.exitCode == 3 || *timer.exitCode == 4) && timerCtx.Err() == nil && !errors.Is(timer.err, exec.ErrWaitDelay) {
			show := runStatusCommand(timerCtx, limits.drain, "systemctl", "--user", "show", unit, "--property=LoadState,ActiveState,SubState", "--no-pager")
			normal = normalUnitStatus(timer, show, unit)
			if show.err != nil && !normal {
				r.Diagnostics = append(r.Diagnostics, commandDiagnostic("unit_state", show))
			} else if !normal {
				r.Diagnostics = append(r.Diagnostics, statusDiagnostic{Source: "unit_state", Severity: "error", Code: "invalid_response", Message: "Could not confirm the unit state from systemctl show.", Stderr: show.stderr, Output: show.stdout})
			} else if show.stderr != "" {
				r.Diagnostics = append(r.Diagnostics, commandWarning("unit_state", show))
			}
		}
		if !normal {
			r.Diagnostics = append(r.Diagnostics, commandDiagnostic("timer", timer))
		} else if timer.stderr != "" {
			r.Diagnostics = append(r.Diagnostics, commandWarning("timer", timer))
		}
	} else if timer.stderr != "" {
		r.Diagnostics = append(r.Diagnostics, commandWarning("timer", timer))
	}
	cancel()
	journalCtx, cancel := context.WithTimeout(context.Background(), limits.journal)
	journal := runStatusCommand(journalCtx, limits.drain, "journalctl", "--user", "-u", job.UnitPrefix+"-"+id+".service", "--no-pager", "-n", "40")
	cancel()
	r.journalOutput = journal.stdout
	r.Journal = strings.TrimRight(journal.stdout, "\n")
	if journal.err != nil {
		r.Diagnostics = append(r.Diagnostics, commandDiagnostic("journal", journal))
	} else if journal.stderr != "" {
		r.Diagnostics = append(r.Diagnostics, commandWarning("journal", journal))
	}
	return r
}

func runStatusCommand(ctx context.Context, drain time.Duration, name string, args ...string) statusCommand {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = drain
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	result := statusCommand{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		code := cmd.ProcessState.ExitCode()
		result.exitCode = &code
	}
	return result
}

func commandDiagnostic(source string, result statusCommand) statusDiagnostic {
	code := "command_failed"
	switch {
	case errors.Is(result.err, context.DeadlineExceeded):
		code = "timeout"
	case errors.Is(result.err, exec.ErrWaitDelay):
		code = "pipe_timeout"
	case errors.Is(result.err, exec.ErrNotFound), errors.Is(result.err, os.ErrNotExist):
		code = "not_found"
	}
	message := result.err.Error()
	if stderr := strings.TrimSpace(result.stderr); stderr != "" {
		message += ": " + stderr
	}
	return statusDiagnostic{Source: source, Severity: "error", Code: code, Message: message, ExitCode: result.exitCode, Stderr: result.stderr, Output: result.stdout}
}

func commandWarning(source string, result statusCommand) statusDiagnostic {
	return statusDiagnostic{Source: source, Severity: "warning", Code: "command_stderr", Message: strings.TrimSpace(result.stderr), ExitCode: result.exitCode, Stderr: result.stderr}
}

// Status 3/4 can be a normal unit condition. Confirm properties separately;
// transport failures, malformed replies, and unexpected stderr remain errors.
func normalUnitStatus(status, show statusCommand, unit string) bool {
	properties := map[string]string{}
	for _, line := range strings.Split(show.stdout, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			properties[key] = value
		}
	}
	load, active, sub := properties["LoadState"], properties["ActiveState"], properties["SubState"]
	if load == "" || active == "" || sub == "" {
		return false
	}
	if show.err != nil {
		var exit *exec.ExitError
		if !errors.As(show.err, &exit) || exit.ExitCode() != 4 || load != "not-found" {
			return false
		}
	}
	if *status.exitCode == 4 {
		if load != "not-found" {
			return false
		}
		stderr := strings.TrimSpace(status.stderr)
		return stderr == "" || stderr == "Unit "+unit+" could not be found."
	}
	if status.stderr != "" {
		return false
	}
	switch load {
	case "loaded", "masked", "error", "bad-setting":
	default:
		return false
	}
	// A unit can change between status and show, so active states are valid too.
	switch active {
	case "active", "reloading", "inactive", "failed", "activating", "deactivating", "maintenance":
		return true
	}
	return false
}
