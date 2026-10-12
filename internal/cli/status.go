package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dj-skn/schedule-send/internal/job"
)

type statusTimeouts struct{ timer, journal, drain time.Duration }

type statusDiagnostic = job.Diagnostic

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
	in := a.sys.Inspect(a.paths, id, limits.timer, limits.journal, limits.drain)
	r.timerOutput, r.Timer = in.TimerOutput, in.Timer
	r.journalOutput, r.Journal = in.JournalOutput, in.Journal
	r.Diagnostics = append(r.Diagnostics, in.Diagnostics...)
	return r
}
