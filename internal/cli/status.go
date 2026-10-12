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

	"github.com/dj-skn/schedule-send/internal/job"
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
	timer := a.sys.Capture(timerCtx, limits.drain, "status", unit, "--no-pager")
	r.timerOutput = timer.Stdout
	r.Timer = strings.TrimRight(timer.Stdout+timer.Stderr, "\n")
	if timer.Err != nil {
		normal := false
		if timer.ExitCode != nil && (*timer.ExitCode == 3 || *timer.ExitCode == 4) && timerCtx.Err() == nil && !errors.Is(timer.Err, exec.ErrWaitDelay) {
			show := a.sys.Capture(timerCtx, limits.drain, "show", unit, "--property=LoadState,ActiveState,SubState", "--no-pager")
			normal = normalUnitStatus(timer, show, unit)
			if show.Err != nil && !normal {
				r.Diagnostics = append(r.Diagnostics, commandDiagnostic("unit_state", show))
			} else if !normal {
				r.Diagnostics = append(r.Diagnostics, statusDiagnostic{Source: "unit_state", Severity: "error", Code: "invalid_response", Message: "Could not confirm the unit state from systemctl show.", Stderr: show.Stderr, Output: show.Stdout})
			} else if show.Stderr != "" {
				r.Diagnostics = append(r.Diagnostics, commandWarning("unit_state", show))
			}
		}
		if !normal {
			r.Diagnostics = append(r.Diagnostics, commandDiagnostic("timer", timer))
		} else if timer.Stderr != "" {
			r.Diagnostics = append(r.Diagnostics, commandWarning("timer", timer))
		}
	} else if timer.Stderr != "" {
		r.Diagnostics = append(r.Diagnostics, commandWarning("timer", timer))
	}
	cancel()
	journalCtx, cancel := context.WithTimeout(context.Background(), limits.journal)
	journal := job.Run(journalCtx, limits.drain, "journalctl", "--user", "-u", job.UnitPrefix+"-"+id+".service", "--no-pager", "-n", "40")
	cancel()
	r.journalOutput = journal.Stdout
	r.Journal = strings.TrimRight(journal.Stdout, "\n")
	if journal.Err != nil {
		r.Diagnostics = append(r.Diagnostics, commandDiagnostic("journal", journal))
	} else if journal.Stderr != "" {
		r.Diagnostics = append(r.Diagnostics, commandWarning("journal", journal))
	}
	return r
}

func commandDiagnostic(source string, result job.Result) statusDiagnostic {
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
	return statusDiagnostic{Source: source, Severity: "error", Code: code, Message: message, ExitCode: result.ExitCode, Stderr: result.Stderr, Output: result.Stdout}
}

func commandWarning(source string, result job.Result) statusDiagnostic {
	return statusDiagnostic{Source: source, Severity: "warning", Code: "command_stderr", Message: strings.TrimSpace(result.Stderr), ExitCode: result.ExitCode, Stderr: result.Stderr}
}

// Status 3/4 can be a normal unit condition. Confirm properties separately;
// transport failures and malformed replies remain errors. Exit 3 means
// systemctl reached the manager and printed the unit, so its stderr (such as
// "changed on disk") is a warning once show confirms the state.
func normalUnitStatus(status, show job.Result, unit string) bool {
	properties := map[string]string{}
	for _, line := range strings.Split(show.Stdout, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			properties[key] = value
		}
	}
	load, active, sub := properties["LoadState"], properties["ActiveState"], properties["SubState"]
	if load == "" || active == "" || sub == "" {
		return false
	}
	if show.Err != nil {
		var exit *exec.ExitError
		if !errors.As(show.Err, &exit) || exit.ExitCode() != 4 || load != "not-found" {
			return false
		}
	}
	if *status.ExitCode == 4 {
		if load != "not-found" {
			return false
		}
		stderr := strings.TrimSpace(status.Stderr)
		return stderr == "" || stderr == "Unit "+unit+" could not be found."
	}
	// A failed or still-active unit whose file is gone loads as not-found
	// and is printed with exit 3 rather than 4.
	switch load {
	case "loaded", "not-found", "masked", "error", "bad-setting":
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
