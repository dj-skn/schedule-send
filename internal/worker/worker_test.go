package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"schedule-send/internal/herdr"
	"schedule-send/internal/job"
	"schedule-send/internal/target"
)

// fake is an agent pane as the worker sees it through herdr. A submit clears
// the composer and starts a turn, unless the agent drops the Enter.
type fake struct {
	rows    []target.Row
	listErr error
	status  string
	seq     int64
	draft   string
	history string // the transcript above the composer
	recent  string
	focused string
	// noRule draws a Codex-style composer with no rule under it, which leaves
	// the emptiness detector off.
	noRule bool
	// codex draws Codex's screen: an input box reading "» Ask Codex to do
	// anything" when empty and the draft after this marker when not, and a
	// submit echoed into the history as "› <message>".
	codex string

	readErr   error
	promptOut string
	promptErr bool
	keysErr   bool
	runErr    bool
	// drops is how many Enter presses the agent ignores, the prompt's own
	// included.
	drops int
	// silent submits leave no trace in the lifecycle.
	silent   bool
	onSubmit func(f *fake)
	onFocus  func(f *fake)

	calls []string
	notes []string
}

func (f *fake) Targets() ([]target.Row, error) { return f.rows, f.listErr }

func (f *fake) Read(pane, source string, lines int) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if source != "visible" {
		return f.recent, nil
	}
	if f.noRule {
		return f.history + "\n› " + f.draft + "\n\n  gpt-5.5 high · 5h 100% left\n", nil
	}
	if f.codex != "" {
		box := "» Ask Codex to do anything"
		if f.draft != "" {
			box = f.codex + " " + f.draft
		}
		return f.history + "\n" + box + "\n\n  GPT-6.1-Sol ultra · ~/projects/shop-backend · 5h 100% left\n", nil
	}
	rule := strings.Repeat("─", 60)
	return f.history + "\n" + rule + "\n❯ " + f.draft + "\n" + rule +
		"\n  ~/projects/a  main  Opus 5.5\n  ctx:16%  session:28% (03:44)  cost:$13.78\n", nil
}

func (f *fake) State(pane string) (string, int64) { return f.status, f.seq }

func (f *fake) Prompt(pane, msg string) (string, error) {
	f.calls = append(f.calls, "prompt "+pane+" "+msg)
	if f.promptErr {
		return f.promptOut, errors.New("exit status 1")
	}
	f.draft = msg
	f.press()
	return "", nil
}

func (f *fake) press() {
	if f.drops > 0 {
		f.drops--
		return
	}
	if f.draft == "" {
		return
	}
	if f.codex != "" {
		f.history += "\n› " + f.draft
	}
	f.draft = ""
	if !f.silent {
		f.seq++
		f.status = "working"
	}
	if f.onSubmit != nil {
		f.onSubmit(f)
	}
}

func (f *fake) SendKeys(surface, pane string, keys []string, _ io.Writer) error {
	f.calls = append(f.calls, surface+" send-keys "+pane+" "+strings.Join(keys, " "))
	if f.keysErr {
		return errors.New("exit status 1")
	}
	if slices.Equal(keys, []string{"enter"}) {
		f.press()
	}
	return nil
}

func (f *fake) Run(pane, msg string, _ io.Writer) error {
	f.calls = append(f.calls, "pane run "+pane+" "+msg)
	if f.runErr {
		return errors.New("exit status 1")
	}
	return nil
}

func (f *fake) Wait(pane string, timeoutMs int) error {
	f.calls = append(f.calls, "wait "+pane)
	return nil
}

func (f *fake) Focus(pane string) error {
	f.calls = append(f.calls, "focus "+pane)
	f.focused = pane
	if f.onFocus != nil {
		f.onFocus(f)
	}
	return nil
}

func (f *fake) Focused() string { return f.focused }

func (f *fake) Notify(title, body string) { f.notes = append(f.notes, title+": "+body) }

// clock is virtual time: sleeping advances it and costs nothing.
type clock struct{ now, start time.Time }

func (c *clock) Now() time.Time        { return c.now }
func (c *clock) Sleep(d time.Duration) { c.now = c.now.Add(d) }
func (c *clock) elapsed() time.Duration {
	return c.now.Sub(c.start)
}

type harness struct {
	t      *testing.T
	f      *fake
	clock  *clock
	paths  job.Paths
	out    bytes.Buffer
	err    bytes.Buffer
	sysLog string
	herdr  string
	code   int
}

const jobID = "20261003-160100-wBp1"

func agentJob() job.State {
	return job.State{
		ID: jobID, Kind: "agent", PaneID: "wB:p1", Agent: "claude", Cwd: "/home/dev/projects/a",
		At: "2026-10-03 16:01:00", Mode: "text", IfBusy: "wait", Recover: "keys", Grace: 4,
		Unit: "herdr-send-" + jobID,
	}
}

func codexJob() job.State {
	st := agentJob()
	st.PaneID, st.Agent, st.Cwd = "wB:p3", "codex", "/home/dev/projects/shop-backend"
	return st
}

func newHarness(t *testing.T) *harness {
	dir := t.TempDir()
	h := &harness{
		t:      t,
		paths:  job.Paths{StateDir: dir + "/state", UnitDir: dir + "/units"},
		sysLog: dir + "/systemctl.log",
		herdr:  dir + "/herdr",
		f: &fake{
			status: "idle", seq: 7,
			history: "● Done. The tests pass.",
			rows: []target.Row{
				{Kind: "agent", Pane: "wA:p1", Agent: "claude", Status: "idle", Cwd: "/home/dev/projects/b", Title: "-"},
				{Kind: "agent", Pane: "wB:p1", Agent: "claude", Status: "idle", Cwd: "/home/dev/projects/a", Title: "-"},
				{Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend", Title: "-"},
				{Kind: "pane", Pane: "wB:p5", Agent: "-", Status: "-", Cwd: "/home/dev/projects/a", Title: "-"},
			},
		},
	}
	h.clock = &clock{now: time.Unix(1791036060, 0)}
	h.clock.start = h.clock.now

	bin := dir + "/bin"
	standin, err := filepath.Abs("../../testdata/standin/systemctl")
	must(t, err)
	must(t, os.Mkdir(bin, 0o755))
	must(t, os.Symlink(standin, bin+"/systemctl"))
	must(t, os.WriteFile(h.herdr, []byte("#!/bin/sh\n"), 0o755))
	must(t, os.MkdirAll(h.paths.UnitDir, 0o755))
	t.Setenv("PATH", bin)
	t.Setenv("SYSTEMCTL_STANDIN_LOG", h.sysLog)
	return h
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// arm writes the job as the scheduler would, with the stored herdr path
// pointing at an executable file. msg nil writes no message file.
func (h *harness) arm(st job.State, msg *string) {
	if st.HerdrBin == "" {
		st.HerdrBin = h.herdr
	}
	must(h.t, h.paths.Write(st, msg))
	for _, ext := range []string{"service", "timer"} {
		must(h.t, os.WriteFile(h.paths.UnitPath(st.ID, ext), nil, 0o644))
	}
}

func (h *harness) fire() {
	w := &Worker{
		ID: jobID, Paths: h.paths, Out: &h.out, Err: &h.err, Clock: h.clock,
		Connect: func(bin string) Herdr {
			if bin != h.herdr {
				h.t.Errorf("connected to %q, want the stored %q", bin, h.herdr)
			}
			return h.f
		},
	}
	h.code = w.Run()
}

func text(s string) *string { return &s }

func (h *harness) wantCode(code int) {
	h.t.Helper()
	if h.code != code {
		h.t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", h.code, code, h.out.String(), h.err.String())
	}
}

func (h *harness) wantOut(want string) {
	h.t.Helper()
	if got := h.out.String(); got != want {
		h.t.Errorf("stdout = %q, want %q", got, want)
	}
}

func (h *harness) wantErr(want string) {
	h.t.Helper()
	if got := h.err.String(); got != want {
		h.t.Errorf("stderr = %q, want %q", got, want)
	}
}

// wantFailure checks a failed fire: the message on stderr, the same message
// as a notification unless quiet, and the job removed.
func (h *harness) wantFailure(msg string) {
	h.t.Helper()
	h.wantCode(1)
	if got := h.err.String(); !strings.HasSuffix(got, msg+"\n") {
		h.t.Errorf("stderr = %q, want it to end with %q", got, msg)
	}
	if want := []string{"Scheduled send failed: " + msg}; !slices.Equal(h.f.notes, want) {
		h.t.Errorf("notifications = %q, want %q", h.f.notes, want)
	}
	h.wantRemoved()
}

func (h *harness) wantRemoved() {
	h.t.Helper()
	for _, p := range []string{
		h.paths.StatePath(jobID), h.paths.MessagePath(jobID),
		h.paths.UnitPath(jobID, "timer"), h.paths.UnitPath(jobID, "service"),
	} {
		if _, err := os.Stat(p); err == nil {
			h.t.Errorf("%s is still there", filepath.Base(p))
		}
	}
	log, _ := os.ReadFile(h.sysLog)
	want := "--user stop herdr-send-" + jobID + ".timer\n--user disable herdr-send-" + jobID + ".timer\n--user daemon-reload\n"
	if string(log) != want {
		h.t.Errorf("systemctl calls = %q, want %q", log, want)
	}
}

func (h *harness) wantCalls(want ...string) {
	h.t.Helper()
	if !slices.Equal(h.f.calls, want) {
		h.t.Errorf("herdr calls:\n  %s\nwant:\n  %s", strings.Join(h.f.calls, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestPromptTakenFirstTime(t *testing.T) {
	h := newHarness(t)
	h.arm(agentJob(), text("carry on with the plan please\n"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1\n")
	h.wantErr("")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")
	h.wantRemoved()
	if len(h.f.notes) != 0 {
		t.Errorf("notifications on success: %q", h.f.notes)
	}
	if got := h.clock.elapsed(); got != limitWatch {
		t.Errorf("took %v, want the %v limit watch", got, limitWatch)
	}
}

func TestDroppedEnterIsRetried(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 1
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1 (Enter retried)\n")
	h.wantErr("Enter was dropped by the agent in wB:p1; retrying the submit.\n")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please", "agent send-keys wB:p1 enter")
}

func TestFocusRungHandsFocusBack(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 2
	h.f.focused = "wA:p1"
	st := agentJob()
	st.Recover = "focus"
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1 (focused, Enter retried)\n")
	h.wantErr("Enter was dropped by the agent in wB:p1; retrying the submit.\n" +
		"Retried Enter did not land in wB:p1; focusing the pane and trying again.\n")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please",
		"agent send-keys wB:p1 enter", "focus wB:p1", "agent send-keys wB:p1 enter", "focus wA:p1")
}

func TestRecoveryFocusTransitions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		onFocus func(*fake)
		failure string
		retry   bool
	}{
		{
			name:    "dialog with draft",
			onFocus: func(f *fake) { f.status = "blocked" },
			failure: "Agent in wB:p1 opened a dialog with the text unsent; it is left in the composer.",
		},
		{
			name:    "unreadable screen",
			onFocus: func(f *fake) { f.readErr = errors.New("screen unavailable") },
			failure: "Text is still unsent in wB:p1's composer: ",
		},
		{
			name:    "draft submitted during focus",
			onFocus: func(f *fake) { f.press() },
		},
		{
			name:    "unknown lifecycle with draft",
			onFocus: func(f *fake) { f.status = "unknown" },
			retry:   true,
		},
		{
			name:    "unreadable lifecycle with draft",
			onFocus: func(f *fake) { f.status = herdr.Unreadable },
			failure: "Text is still unsent in wB:p1's composer:  ❯ carry on with the plan please",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.f.drops = 2
			h.f.focused = "wA:p1"
			h.f.onFocus = func(f *fake) {
				if f.focused == "wB:p1" {
					tc.onFocus(f)
				}
			}
			st := agentJob()
			st.Recover = "focus"
			h.arm(st, text("carry on with the plan please"))
			h.fire()
			if tc.failure != "" {
				h.wantFailure(tc.failure)
			} else {
				h.wantCode(0)
			}
			calls := []string{
				"wait wB:p1", "prompt wB:p1 carry on with the plan please",
				"agent send-keys wB:p1 enter", "focus wB:p1",
			}
			if tc.retry {
				calls = append(calls, "agent send-keys wB:p1 enter")
			}
			calls = append(calls, "focus wA:p1")
			h.wantCalls(calls...)
			if h.f.focused != "wA:p1" {
				t.Errorf("focused pane = %q, want wA:p1", h.f.focused)
			}
			if tc.name == "dialog with draft" && h.f.draft != "carry on with the plan please" {
				t.Errorf("dialog draft = %q, want original text", h.f.draft)
			}
		})
	}
}

func TestLadderExhaustedQuotesTheDraft(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 5
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Text is still unsent in wB:p1's composer:  ❯ carry on with the plan please")
	// The prompt's own window, then one retry's: each runs to the grace deadline.
	if got := h.clock.elapsed(); got != 8*time.Second {
		t.Errorf("took %v, want two 4s grace windows", got)
	}
}

func TestRecoverNoneFailsWithoutRetrying(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 1
	st := agentJob()
	st.Recover = "none"
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Text is still unsent in wB:p1's composer:  ❯ carry on with the plan please")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")
}

func TestDialogIsLeftAlone(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 1
	h.f.status = "blocked"
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Agent in wB:p1 opened a dialog with the text unsent; it is left in the composer.")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")
}

// A failed lifecycle read is no evidence that a dialog is closed, so the
// recovery Enter is withheld, while a reported "unknown" status still retries.
func TestUnreadableLifecycleWithholdsEnter(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 1
	h.f.status = herdr.Unreadable
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Text is still unsent in wB:p1's composer:  ❯ carry on with the plan please")
	if !strings.Contains(h.err.String(), "Could not read the state of wB:p1, so Enter is not retried.\n") {
		t.Errorf("stderr = %q", h.err.String())
	}
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")

	h = newHarness(t)
	h.f.drops = 1
	h.f.status = "unknown"
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantCode(0)
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please", "agent send-keys wB:p1 enter")
}

func TestUnverifiableSendIsReportedNotGuessed(t *testing.T) {
	h := newHarness(t)
	h.f.noRule = true
	h.f.silent = true
	h.arm(agentJob(), text("go"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1 (unverified)\n")
	h.wantErr("Note: neither submit detector applies to wB:p1; the send cannot be verified.\n" +
		"Delivered to wB:p1 but the submit is unverified: no draft in its composer and no state change.\n")
	h.wantCalls("wait wB:p1", "prompt wB:p1 go")
	if got := h.clock.elapsed(); got != 4*time.Second+limitWatch {
		t.Errorf("took %v, want the grace window and the limit watch", got)
	}
}

func TestCodexEchoIsNotADraft(t *testing.T) {
	// The echo "› carry on…" sits above the "»" input box; taking it for the
	// composer would read a sent message as unsent and press Enter again.
	h := newHarness(t)
	h.f.codex = "»"
	h.arm(codexJob(), text("carry on with the plan please"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p3\n")
	h.wantErr("")
	h.wantCalls("wait wB:p3", "prompt wB:p3 carry on with the plan please")
	h.wantRemoved()
}

func TestCodexDraftStillSeen(t *testing.T) {
	// Which marker Codex draws before a typed draft is unconfirmed.
	for _, marker := range []string{"»", "›"} {
		t.Run(marker, func(t *testing.T) {
			h := newHarness(t)
			h.f.codex = marker
			h.f.drops = 5
			h.arm(codexJob(), text("carry on with the plan please"))
			h.fire()
			h.wantFailure("Text is still unsent in wB:p3's composer:  " + marker +
				" carry on with the plan please GPT-6.1-Sol ultra · ~/projects/shop-backend · 5h 100% left")
			h.wantCalls("wait wB:p3", "prompt wB:p3 carry on with the plan please", "agent send-keys wB:p3 enter")
		})
	}
}

func TestStillLimitedAfterSubmit(t *testing.T) {
	h := newHarness(t)
	old := "  ⎿  You've hit your weekly limit · resets 4pm (Europe/Warsaw)\n" +
		"     /upgrade to increase your usage limit.\n"
	h.f.history = old
	h.f.onSubmit = func(f *fake) {
		f.history = old + "\n❯ resume\n" + old
	}
	h.arm(agentJob(), text("resume"))
	h.fire()
	h.wantFailure("Agent in wB:p1 is still usage-limited; the prompt was not acted on: " +
		" ⎿ You've hit your weekly limit · resets 4pm (Europe/Warsaw) /upgrade to increase your usage limit.")
	if got := h.clock.elapsed(); got != time.Second {
		t.Errorf("took %v, want the first watch poll", got)
	}
}

func TestNoticeAboveTheMessageIsTheOldLimit(t *testing.T) {
	h := newHarness(t)
	old := "  ⎿  You've hit your weekly limit · resets 4pm (Europe/Warsaw)\n"
	h.f.history = old
	h.f.onSubmit = func(f *fake) { f.recent = old + "\n❯ resume\n\n● Picking up where I left off.\n" }
	h.arm(agentJob(), text("resume"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1\n")
}

func TestPromptRefused(t *testing.T) {
	for _, c := range []struct{ out, detail string }{
		{`{"error":{"code":"agent_not_found","message":"no agent in pane wB:p1"}}` + "\n", "no agent in pane wB:p1"},
		{"connection refused\nretry later\n", "connection refused"},
		{"", "exit status 1"},
	} {
		h := newHarness(t)
		h.f.promptErr = true
		h.f.promptOut = c.out
		h.arm(agentJob(), text("carry on with the plan please"))
		h.fire()
		h.wantFailure("agent prompt to wB:p1 was refused: " + c.detail)
	}
}

func TestFireTimeValidation(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(rows []target.Row) []target.Row
		msg  string
	}{
		{"gone", func(r []target.Row) []target.Row { return r[:1] }, "Target pane wB:p1 no longer exists; send aborted."},
		{"shell", func(r []target.Row) []target.Row {
			r[1] = target.Row{Kind: "pane", Pane: "wB:p1", Agent: "-", Status: "-", Cwd: "/home/dev/projects/a", Title: "-"}
			return r
		}, "Pane wB:p1 is no longer a agent; send aborted."},
		{"other agent", func(r []target.Row) []target.Row { r[1].Agent = "codex"; return r },
			"Pane wB:p1 now runs codex, not claude; send aborted."},
		{"moved", func(r []target.Row) []target.Row { r[1].Cwd = "/tmp"; return r },
			"Pane wB:p1 moved to /tmp; send aborted."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.f.rows = c.edit(h.f.rows)
			h.arm(agentJob(), text("carry on with the plan please"))
			h.fire()
			h.wantFailure(c.msg)
			h.wantCalls()
		})
	}
}

func TestHerdrUnreadableAtFireTime(t *testing.T) {
	h := newHarness(t)
	h.f.listErr = errors.New("exit status 1")
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Could not list herdr's panes to check wB:p1; send aborted.")
}

func TestForceSkipsValidation(t *testing.T) {
	h := newHarness(t)
	h.f.rows = nil
	st := agentJob()
	st.Force = 1
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1\n")
}

func TestQuietFailureRaisesNoNotification(t *testing.T) {
	h := newHarness(t)
	h.f.rows = nil
	st := agentJob()
	st.Quiet = 1
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantCode(1)
	h.wantErr("Target pane wB:p1 no longer exists; send aborted.\n")
	if len(h.f.notes) != 0 {
		t.Errorf("notifications = %q", h.f.notes)
	}
	h.wantRemoved()
}

func TestBusyAgent(t *testing.T) {
	h := newHarness(t)
	h.f.status = "working"
	st := agentJob()
	st.IfBusy = "abort"
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Agent in wB:p1 is working and --if-busy=abort was set.")
	h.wantCalls()

	h = newHarness(t)
	h.f.status = "working"
	h.arm(agentJob(), text("carry on with the plan please"))
	h.fire()
	h.wantFailure("Agent in wB:p1 was still working after 15 minutes; send aborted.")
	h.wantCalls("wait wB:p1")
}

func TestKeys(t *testing.T) {
	h := newHarness(t)
	st := agentJob()
	st.Mode, st.Keys = "keys", "ctrl+c  enter"
	h.arm(st, nil)
	h.fire()
	h.wantCode(0)
	h.wantOut("sent keys to wB:p1: ctrl+c  enter\n")
	h.wantCalls("agent send-keys wB:p1 ctrl+c enter")
	h.wantRemoved()

	h = newHarness(t)
	st = job.State{ID: jobID, Kind: "pane", PaneID: "wB:p5", Agent: "-", Cwd: "/home/dev/projects/a",
		Mode: "keys", Keys: "esc", IfBusy: "wait", Recover: "keys", Grace: 4}
	h.arm(st, nil)
	h.fire()
	h.wantCode(0)
	h.wantCalls("pane send-keys wB:p5 esc")

	h = newHarness(t)
	h.f.keysErr = true
	h.arm(st, nil)
	h.fire()
	h.wantFailure("send-keys to wB:p5 failed.")
}

func TestPaneRun(t *testing.T) {
	h := newHarness(t)
	st := job.State{ID: jobID, Kind: "pane", PaneID: "wB:p5", Agent: "-", Cwd: "/home/dev/projects/a",
		Mode: "text", IfBusy: "wait", Recover: "keys", Grace: 4}
	h.arm(st, text("make test\n\n"))
	h.fire()
	h.wantCode(0)
	h.wantOut("ran text in wB:p5\n")
	h.wantCalls("pane run wB:p5 make test")

	h = newHarness(t)
	h.f.runErr = true
	h.arm(st, text("make test"))
	h.fire()
	h.wantFailure("pane run in wB:p5 failed.")
}

func TestMissingMessageFails(t *testing.T) {
	h := newHarness(t)
	h.arm(agentJob(), nil)
	h.fire()
	h.wantFailure("The message for " + jobID + " is missing; send aborted.")
}

func TestHerdrGoneAtFireTime(t *testing.T) {
	h := newHarness(t)
	st := agentJob()
	st.HerdrBin = "/nonexistent/herdr"
	h.arm(st, text("carry on with the plan please"))
	h.fire()
	h.wantCode(1)
	h.wantErr("herdr binary is unavailable at fire time.\n")
	h.wantRemoved()
}

func TestNoStateFile(t *testing.T) {
	h := newHarness(t)
	h.fire()
	h.wantCode(1)
	h.wantErr("No state file for " + jobID + "\n")
	if _, err := os.Stat(h.sysLog); err == nil {
		t.Error("systemctl was called")
	}
}

// A timer armed before recover and grace existed still fires, with the
// defaults.
func TestStateFileFromAnOlderVersion(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 1
	must(t, os.MkdirAll(h.paths.StateDir, 0o700))
	old := `{"id":"` + jobID + `","kind":"agent","pane_id":"wB:p1","agent":"claude",` +
		`"cwd":"/home/dev/projects/a","at":"2026-10-03 16:01:00","mode":"text","keys":"",` +
		`"if_busy":"wait","force":0,"quiet":0,"herdr_bin":"` + h.herdr + `","unit":"herdr-send-` + jobID + `"}`
	must(t, os.WriteFile(h.paths.StatePath(jobID), []byte(old), 0o644))
	must(t, os.WriteFile(h.paths.MessagePath(jobID), []byte("carry on with the plan please"), 0o644))
	h.fire()
	h.wantCode(0)
	h.wantOut("prompted agent in wB:p1 (Enter retried)\n")
}

type failingScreen struct {
	*fake
	reads int
	fail  bool
}

func (f *failingScreen) Read(pane, source string, lines int) (string, error) {
	if source == "visible" {
		f.reads++
		if f.reads >= 3 {
			if f.fail {
				return "", errors.New("read failed")
			}
			return "", nil
		}
	}
	return f.fake.Read(pane, source, lines)
}
func TestUnreadableScreenDoesNotConfirmDraft(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			h := newHarness(t)
			h.f.drops = 5
			h.arm(agentJob(), text("carry on with the plan please"))
			f := &failingScreen{fake: h.f, fail: fail}
			w := &Worker{ID: jobID, Paths: h.paths, Out: &h.out, Err: &h.err, Clock: h.clock, Connect: func(string) Herdr { return f }}
			h.code = w.Run()
			h.wantFailure("Could not verify the draft in wB:p1 after its screen became unreadable; send aborted.")
			h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")
			if h.f.draft == "" {
				t.Fatal("draft was unexpectedly submitted")
			}
		})
	}
}

type movingAgent struct{ *fake }

func (f *movingAgent) Wait(pane string, timeout int) error {
	_ = f.fake.Wait(pane, timeout)
	f.rows[1].Agent = "codex"
	f.rows[1].Cwd = "/tmp/different-project"
	return nil
}
func TestRevalidatesTargetAfterWait(t *testing.T) {
	h := newHarness(t)
	h.arm(agentJob(), text("carry on with the plan please"))
	f := &movingAgent{fake: h.f}
	w := &Worker{ID: jobID, Paths: h.paths, Out: &h.out, Err: &h.err, Clock: h.clock, Connect: func(string) Herdr { return f }}
	h.code = w.Run()
	h.wantFailure("Pane wB:p1 now runs codex, not claude; send aborted.")
	h.wantCalls("wait wB:p1")
}

func TestInvalidStateDoesNotSend(t *testing.T) {
	h := newHarness(t)
	st := agentJob()
	h.arm(st, text("carry on with the plan please"))
	st.Mode = "invalid"
	must(t, os.WriteFile(h.paths.StatePath(jobID), job.Encode(st), 0600))
	h.fire()
	h.wantCode(1)
	h.wantCalls()
	if !strings.Contains(h.err.String(), "Invalid state file") {
		t.Fatalf("stderr=%s", h.err.String())
	}
}

func TestCleanupFailureMakesSuccessfulSendNonzero(t *testing.T) {
	h := newHarness(t)
	h.arm(agentJob(), text("carry on with the plan please"))
	sys := filepath.Join(filepath.Dir(h.sysLog), "bin", "systemctl")
	// Replace the local symlink, never write through it into the shared stand-in.
	must(t, os.Remove(sys))
	must(t, os.WriteFile(sys, []byte("#!/bin/sh\necho unavailable >&2\nexit 1\n"), 0755))
	h.fire()
	h.wantCode(1)
	if !strings.Contains(h.err.String(), "was sent but cleanup failed") {
		t.Fatalf("stderr=%s", h.err.String())
	}
	if _, err := os.Stat(h.paths.StatePath(jobID)); err != nil {
		t.Fatalf("state was discarded: %v", err)
	}
}

type changedComposer struct {
	*fake
	reads int
}

func (f *changedComposer) Read(pane, source string, lines int) (string, error) {
	if source == "visible" {
		f.reads++
		if f.reads >= 3 {
			return "› transformed draft\n footer", nil
		}
	}
	return f.fake.Read(pane, source, lines)
}
func TestChangedComposerDoesNotConfirmDraftDisappearance(t *testing.T) {
	h := newHarness(t)
	h.f.drops = 5
	h.arm(agentJob(), text("carry on with the plan please"))
	f := &changedComposer{fake: h.f}
	w := &Worker{ID: jobID, Paths: h.paths, Out: &h.out, Err: &h.err, Clock: h.clock, Connect: func(string) Herdr { return f }}
	h.code = w.Run()
	h.wantFailure("Could not verify the draft in wB:p1 after its screen became unreadable; send aborted.")
	h.wantCalls("wait wB:p1", "prompt wB:p1 carry on with the plan please")
	if h.f.draft == "" {
		t.Fatal("draft was unexpectedly submitted")
	}
}

func TestMalformedNumericStateNeverConnects(t *testing.T) {
	for _, kind := range []string{"agent text", "agent keys", "pane text"} {
		for _, tc := range []struct{ field, value string }{
			{"force", `"1"`}, {"force", "2"},
			{"quiet", "true"}, {"quiet", "-1"},
			{"grace", "-1"}, {"grace", "4.5"}, {"grace", "4.0"}, {"grace", "4e0"}, {"grace", "0"}, {"grace", "61"},
			{"run_at", "-1"}, {"run_at", "9223372036854775808"},
		} {
			t.Run(kind+"/"+tc.field+"="+tc.value, func(t *testing.T) {
				h := newHarness(t)
				state := agentJob()
				switch kind {
				case "agent keys":
					state.Mode = "keys"
					state.Keys = "enter"
				case "pane text":
					state.Kind = "pane"
					state.PaneID = "wB:p5"
					state.Agent = "-"
				}
				h.arm(state, text("carry on with the plan please"))
				stored, err := os.ReadFile(h.paths.StatePath(jobID))
				must(t, err)
				var raw map[string]json.RawMessage
				must(t, json.Unmarshal(stored, &raw))
				raw[tc.field] = json.RawMessage(tc.value)
				data, err := json.Marshal(raw)
				must(t, err)
				must(t, os.WriteFile(h.paths.StatePath(jobID), data, 0600))
				connections := 0
				w := &Worker{ID: jobID, Paths: h.paths, Out: &h.out, Err: &h.err, Clock: h.clock, Connect: func(string) Herdr { connections++; return h.f }}
				h.code = w.Run()
				h.wantCode(1)
				h.wantCalls()
				if connections != 0 {
					t.Fatalf("connected to Herdr %d times", connections)
				}
				if !strings.Contains(h.err.String(), "job field "+tc.field) {
					t.Fatalf("stderr: %s", h.err.String())
				}
				stored, err = os.ReadFile(h.paths.StatePath(jobID))
				must(t, err)
				if string(stored) != string(data) {
					t.Fatal("malformed state was changed or removed")
				}
				for _, path := range []string{h.paths.MessagePath(jobID), h.paths.UnitPath(jobID, "timer"), h.paths.UnitPath(jobID, "service")} {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("job artifact removed: %s: %v", path, err)
					}
				}
				if calls, err := os.ReadFile(h.sysLog); err == nil && len(calls) != 0 {
					t.Fatalf("systemctl calls on invalid state: %s", calls)
				}
			})
		}
	}
}
