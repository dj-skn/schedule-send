package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"schedule-send/internal/target"
)

// standin points a client at the stand-in herdr, serving a copy of the saved
// 2026-10-03 screens so a test can add files without touching the originals.
func standin(t *testing.T) (Client, string) {
	t.Helper()
	dir := t.TempDir()
	src := "../../testdata/fixtures/2026-10-03"
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HERDR_STANDIN_DIR", dir)
	bin, err := filepath.Abs("../../testdata/standin/herdr")
	if err != nil {
		t.Fatal(err)
	}
	return Client{Bin: bin}, dir
}

func TestTargets(t *testing.T) {
	c, _ := standin(t)
	rows, err := c.Targets()
	if err != nil {
		t.Fatal(err)
	}
	want := []target.Row{
		{Kind: "agent", Pane: "wA:p1", Agent: "claude", Status: "idle", Cwd: "/home/dev/projects/analytics", Title: "Analytics backend rewrite"},
		{Kind: "agent", Pane: "wB:p1", Agent: "claude", Status: "idle", Cwd: "/home/dev/projects/shop-backend", Title: "checkout-webhook-migration"},
		{Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend", Title: "Implement SB-42 phase 3.1 | shop-backend"},
		{Kind: "agent", Pane: "wE:p1", Agent: "cursor", Status: "working", Cwd: "/home/dev/src/schedule-send", Title: "Tidy schedule-send UI"},
	}
	if len(rows) != 6 {
		t.Fatalf("got %d rows: %+v", len(rows), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
	for _, r := range rows[4:] {
		if r.Kind != "pane" || r.Agent != "-" || r.Status != "-" {
			t.Errorf("plain pane row = %+v", r)
		}
	}
}

func TestStateAndRead(t *testing.T) {
	c, dir := standin(t)
	if st, seq := c.State("wB:p3"); st != "idle" || seq != 218 {
		t.Errorf("State(wB:p3) = %s %d", st, seq)
	}
	if st, seq := c.State("wZ:p9"); st != Unreadable || seq != -1 {
		t.Errorf("State of a missing agent = %s %d", st, seq)
	}
	if out, err := c.Read("wB:p1", "visible", 40); err != nil || !strings.Contains(out, "weekly limit") {
		t.Errorf("Read visible = %q", out)
	}
	if out, err := c.Read("wB:p1", "recent-unwrapped", 80); err != nil || out != "" {
		t.Errorf("Read recent with no file = %q", out)
	}
	if got := c.ProcName("wB:p5"); got != "zsh" {
		t.Errorf("ProcName = %q", got)
	}

	if _, err := c.Prompt("wB:p3", "resume"); err != nil {
		t.Fatal(err)
	}
	if st, seq := c.State("wB:p3"); st != "working" || seq != 219 {
		t.Errorf("after prompt = %s %d", st, seq)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	if !strings.Contains(string(log), "agent prompt wB:p3 resume") {
		t.Errorf("calls.log = %s", log)
	}
}

func TestPromptError(t *testing.T) {
	if got := PromptError(`{"error":{"code":"agent_blocked","message":"agent is blocked"}}`); got != "agent is blocked" {
		t.Errorf("json: %q", got)
	}
	if got := PromptError("plain failure\nmore"); got != "plain failure" {
		t.Errorf("text: %q", got)
	}
}

func TestLocate(t *testing.T) {
	bin, _ := filepath.Abs("../../testdata/standin/herdr")
	t.Setenv("HERDR_BIN_OVERRIDE", bin)
	if got, err := Locate(); err != nil || got != bin {
		t.Errorf("Locate = %q, %v", got, err)
	}
	t.Setenv("HERDR_BIN_OVERRIDE", "/no/such/herdr")
	if _, err := Locate(); err == nil || err.Error() != "herdr is not on PATH." {
		t.Errorf("missing override: %v", err)
	}
}

func TestExitZeroJSONErrors(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "herdr")
			redirect := ""
			if stream == "stderr" {
				redirect = " >&2"
			}
			script := "#!/bin/sh\nprintf '%s\\n' '{\"error\":{\"code\":\"agent_not_found\",\"message\":\"no agent\"}}'" + redirect + "\nexit 0\n"
			if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			c := Client{Bin: bin}
			_, promptErr := c.Prompt("gone", "resume")
			_, readErr := c.Read("gone", "visible", 40)
			_, targetsErr := c.Targets()
			errs := map[string]error{
				"Prompt": promptErr, "Targets": targetsErr,
				"SendKeys": c.SendKeys("agent", "gone", []string{"enter"}, nil),
				"Run":      c.Run("gone", "resume", nil), "Wait": c.Wait("gone", 900000), "Focus": c.Focus("gone"),
			}
			if stream == "stderr" {
				errs["Read"] = readErr
			} else if readErr != nil {
				t.Errorf("raw JSON transcript caused Read error: %v", readErr)
			}
			for name, err := range errs {
				if err == nil || err.Error() != "no agent" {
					t.Errorf("%s error=%v, want no agent", name, err)
				}
			}
			if status, seq := c.State("gone"); status != Unreadable || seq != -1 {
				t.Errorf("State=%s %d", status, seq)
			}
		})
	}
}

func TestCommandDeadline(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "herdr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c := Client{Bin: bin, Timeout: 20 * time.Millisecond}
	started := time.Now()
	_, err := c.Prompt("pane", "resume")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	c.Notify("failure", "message")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("commands took %v", elapsed)
	}
}
