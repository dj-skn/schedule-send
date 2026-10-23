//go:build linux

package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const statusID = "20261003-160100-wBp1"

func statusFixture(t *testing.T, h *harness, name, body string) {
	t.Helper()
	path := filepath.Join(h.dir, "path", name)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStatusInspection(t *testing.T) {
	cases := []struct {
		name, timer, journal, state, source, diagnostic string
		exit                                            int
	}{
		{name: "active", timer: "echo timer-partial", journal: "echo journal-partial"},
		{name: "inactive", timer: "if [ \"$2\" = status ]; then echo timer-partial; exit 3; fi\nprintf 'LoadState=loaded\\nActiveState=inactive\\nSubState=dead\\n'", journal: "echo journal-partial"},
		{name: "failed unit", timer: "if [ \"$2\" = status ]; then echo timer-partial; exit 3; fi\nprintf 'LoadState=loaded\\nActiveState=failed\\nSubState=failed\\n'", journal: "echo journal-partial"},
		{name: "missing unit", timer: "if [ \"$2\" = status ]; then echo \"Unit $3 could not be found.\" >&2; exit 4; fi\nprintf 'LoadState=not-found\\nActiveState=inactive\\nSubState=dead\\n'; exit 4", journal: "echo journal-partial"},
		{name: "missing state", timer: "echo timer-partial", journal: "echo journal-partial", state: "missing"},
		{name: "numeric state inspectable", timer: "echo timer-partial", journal: "echo journal-partial", state: "numeric"},
		{name: "bus failure", timer: "echo timer-partial; echo bus-failure >&2; exit 1", journal: "echo journal-partial", source: "timer", diagnostic: "command_failed", exit: 1},
		{name: "inactive changed on disk", timer: "if [ \"$2\" = status ]; then echo timer-partial; echo \"Warning: The unit file, source configuration file or drop-ins of $3 changed on disk.\" >&2; exit 3; fi\nprintf 'LoadState=loaded\\nActiveState=inactive\\nSubState=dead\\n'", journal: "echo journal-partial", source: "timer", diagnostic: "command_stderr"},
		{name: "failed unit file removed", timer: "if [ \"$2\" = status ]; then echo timer-partial; exit 3; fi\nprintf 'LoadState=not-found\\nActiveState=failed\\nSubState=failed\\n'", journal: "echo journal-partial"},
		{name: "invalid unit reply", timer: "if [ \"$2\" = status ]; then echo timer-partial; exit 3; fi\necho nonsense", journal: "echo journal-partial", source: "unit_state", diagnostic: "invalid_response", exit: 1},
		{name: "unit query failure", timer: "if [ \"$2\" = status ]; then echo timer-partial; exit 3; fi\necho bus-failure >&2; exit 1", journal: "echo journal-partial", source: "unit_state", diagnostic: "command_failed", exit: 1},
		{name: "missing systemctl", journal: "echo journal-partial", source: "timer", diagnostic: "not_found", exit: 1},
		{name: "missing journalctl", timer: "echo timer-partial", source: "journal", diagnostic: "not_found", exit: 1},
		{name: "journal failure", timer: "echo timer-partial", journal: "echo journal-partial; echo journal-failure >&2; exit 2", source: "journal", diagnostic: "command_failed", exit: 1},
		{name: "journal warning", timer: "echo timer-partial", journal: "echo journal-partial; echo journal-warning >&2", source: "journal", diagnostic: "command_stderr"},
		{name: "unreadable state", timer: "echo timer-partial", journal: "echo journal-partial", state: "directory", source: "state", diagnostic: "read_failed", exit: 1},
		{name: "invalid JSON", timer: "echo timer-partial", journal: "echo journal-partial", state: "invalid", source: "state", diagnostic: "invalid_json", exit: 1},
	}
	for _, tc := range cases {
		for _, jsonMode := range []bool{false, true} {
			mode := "text"
			if jsonMode {
				mode = "json"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				h := newHarness(t, nil)
				h.addJobs(t)
				t.Setenv("PATH", filepath.Join(h.dir, "path"))
				statusFixture(t, h, "systemctl", tc.timer)
				statusFixture(t, h, "journalctl", tc.journal)
				path := h.a.paths.StatePath(statusID)
				switch tc.state {
				case "missing", "directory":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if tc.state == "directory" {
						if err := os.Mkdir(path, 0755); err != nil {
							t.Fatal(err)
						}
					}
				case "invalid", "numeric":
					content := "broken-json"
					if tc.state == "numeric" {
						content = `{"grace":4.0}`
					}
					if err := os.WriteFile(path, []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{"--status", statusID}
				if jsonMode {
					args = append(args, "--json")
				}
				if code := h.a.main(args); code != tc.exit {
					t.Fatalf("exit %d want %d; stderr %s", code, tc.exit, h.stderr)
				}
				if tc.exit != 0 && !strings.Contains(h.stderr.String(), "Status inspection incomplete") {
					t.Fatalf("missing error: %s", h.stderr)
				}
				if strings.Contains(tc.timer, "echo timer-partial") && !strings.Contains(h.stdout.String(), "timer-partial") {
					t.Fatal("lost timer output")
				}
				if strings.Contains(tc.journal, "echo journal-partial") && !strings.Contains(h.stdout.String(), "journal-partial") {
					t.Fatal("lost journal output")
				}
				if !jsonMode {
					if tc.source != "" && !strings.Contains(h.stderr.String(), tc.source+":") {
						t.Fatalf("missing source: %s", h.stderr)
					}
					return
				}
				var result statusResult
				decoder := json.NewDecoder(h.stdout)
				if err := decoder.Decode(&result); err != nil {
					t.Fatal(err)
				}
				var extra any
				if err := decoder.Decode(&extra); err != io.EOF {
					t.Fatalf("extra stdout result: %v", err)
				}
				if tc.state == "missing" && (string(result.State) != "null" || result.Warning == "") {
					t.Fatalf("missing state: %+v", result)
				}
				if tc.state == "numeric" && string(result.State) != `{"grace":4.0}` {
					t.Fatalf("changed raw state %s", result.State)
				}
				if tc.source != "" {
					found := false
					for _, d := range result.Diagnostics {
						if d.Source == tc.source && d.Code == tc.diagnostic {
							found = true
							severity := "error"
							if tc.exit == 0 {
								severity = "warning"
							}
							if d.Severity != severity {
								t.Errorf("severity %s", d.Severity)
							}
							if tc.name == "journal failure" && (d.ExitCode == nil || *d.ExitCode != 2 || !strings.Contains(d.Stderr, "journal-failure") || !strings.Contains(d.Output, "journal-partial")) {
								t.Errorf("lost evidence: %+v", d)
							}
						}
					}
					if !found {
						t.Fatalf("missing diagnostic: %+v", result.Diagnostics)
					}
				} else {
					for _, d := range result.Diagnostics {
						if d.Severity == "error" {
							t.Errorf("normal condition rejected: %+v", d)
						}
					}
				}
			})
		}
	}
}

// The budgets leave each fixture command about 250 ms to start and print
// before its deadline, so a busy machine does not turn a partial result into
// a missing one. The sleeps that must outlast a budget are far longer than it.
func TestStatusDeadlines(t *testing.T) {
	cases := []struct{ name, timer, journal, source, code string }{
		{"timer deadline", "echo timer-partial; exec /bin/sleep 10", "echo journal-partial", "timer", "timeout"},
		{"journal deadline", "echo timer-partial", "echo journal-partial; exec /bin/sleep 10", "journal", "timeout"},
		{"shared timer budget", "if [ \"$2\" = status ]; then echo timer-partial; /bin/sleep 0.2; exit 3; fi\n/bin/sleep 0.6; printf 'LoadState=loaded\\nActiveState=inactive\\nSubState=dead\\n'", "echo journal-partial", "unit_state", "timeout"},
		{"pipe drain", "echo timer-partial", "echo journal-partial; /bin/sleep 1 &", "journal", "pipe_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.addJobs(t)
			t.Setenv("PATH", filepath.Join(h.dir, "path"))
			statusFixture(t, h, "systemctl", tc.timer)
			statusFixture(t, h, "journalctl", tc.journal)
			h.a.statusTimeouts = statusTimeouts{timer: 500 * time.Millisecond, journal: 500 * time.Millisecond, drain: 100 * time.Millisecond}
			start := time.Now()
			if code := h.a.main([]string{"--status", statusID, "--json"}); code != 1 {
				t.Fatalf("exit %d: %s", code, h.stderr)
			}
			elapsed := time.Since(start)
			if elapsed > 3*time.Second {
				t.Fatalf("inspection took %v", elapsed)
			}
			var result statusResult
			if err := json.Unmarshal(h.stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range result.Diagnostics {
				if d.Source == tc.source && d.Code == tc.code {
					found = true
				}
			}
			if !found || !strings.Contains(result.Timer, "timer-partial") || !strings.Contains(result.Journal, "journal-partial") {
				t.Fatalf("lost partial result/diagnostic: %+v", result)
			}
			t.Logf("completed in %v", elapsed)
		})
	}
}
