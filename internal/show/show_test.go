package show

import (
	"strings"
	"testing"
	"time"

	"github.com/dj-skn/schedule-send/internal/limit"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
)

func warsaw(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	prev := when.Loc
	when.Loc = loc
	t.Cleanup(func() { when.Loc = prev })
	return loc
}

// Sat 2026-10-03 and Sun 2026-10-04, Europe/Warsaw.
const (
	at1522    = 1791033720 // 15:22:00
	at1600    = 1791036000 // 16:00:00
	at161640  = 1791037000 // 16:16:40
	atSun1600 = 1791122400 // Sun 16:00:00
)

// markers stand in for the colour codes, so a test can see where each goes.
var markers = term.Colors{Reset: "</>", Dim: "<dim>", Yellow: "<y>", Green: "<g>", Red: "<r>", Cyan: "<c>"}

// Sends as tools/parity arms them, run at 15:22: auto-own-notice, arm-text,
// arm-keys and enter-only.
var (
	resume = Scheduled{
		ID: "20261003-160100-wBp1", Unit: "herdr-send-20261003-160100-wBp1.timer",
		Kind: "agent", Pane: "wB:p1", Agent: "claude", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
		At: "2026-10-03 16:01:00", RunAt: 1791036060, Delay: 2340,
		Mode: "text", Payload: "resume", Message: "resume",
		IfBusy: "wait", Recover: "keys", Grace: "4",
		UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261003-160100-wBp1.{service,timer}",
		WouldRun:  "/opt/schedule-send __fire 20261003-160100-wBp1",
	}
	twoLines = Scheduled{
		ID: "20261003-163000-wBp3", Unit: "herdr-send-20261003-163000-wBp3.timer",
		Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
		At: "2026-10-03 16:30:00", RunAt: 1791037800, Delay: 4080,
		Mode: "text", Payload: "two", Message: "two\nlines",
		IfBusy: "wait", Recover: "keys", Grace: "4",
		UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261003-163000-wBp3.{service,timer}",
		WouldRun:  "/opt/schedule-send __fire 20261003-163000-wBp3",
	}
	paneKeys = Scheduled{
		ID: "20261004-090000-wBp5", Unit: "herdr-send-20261004-090000-wBp5.timer",
		Kind: "pane", Pane: "wB:p5", Agent: "-", Status: "-", Cwd: "/home/dev/projects/shop-backend",
		At: "2026-10-04 09:00:00", RunAt: 1791097200, Delay: 63480,
		Mode: "keys", Payload: "keys: ctrl+c", Keys: "ctrl+c",
		Persistent: true, IfBusy: "abort", Recover: "focus", Grace: "9",
		UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp5.{service,timer}",
		WouldRun:  "/opt/schedule-send __fire 20261004-090000-wBp5",
	}
	enterOnly = Scheduled{
		DryRun: true, ID: "20261004-090000-wBp3", Unit: "herdr-send-20261004-090000-wBp3.timer",
		Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
		At: "2026-10-04 09:00:00", RunAt: 1791097200, Delay: 63480,
		Mode: "keys", Payload: "keys: enter", Keys: "enter",
		Persistent: true, IfBusy: "wait", Recover: "keys", Grace: "4",
		UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp3.{service,timer}",
		WouldRun:  "/opt/schedule-send __fire 20261004-090000-wBp3",
	}
)

func TestScheduledPlain(t *testing.T) {
	cases := []struct {
		name string
		s    Scheduled
		want string
	}{
		{"dry run", enterOnly, `target=agent wB:p3 (codex, idle)
cwd=/home/dev/projects/shop-backend
at=2026-10-04 09:00:00
run_at_epoch=1791097200
delay_seconds=63480
mode=keys
payload=keys: enter
id=20261004-090000-wBp3
unit=herdr-send-20261004-090000-wBp3.timer
persistent=1
if_busy=wait
recover=keys
submit_grace=4
unit_files=/home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp3.{service,timer}
would_run=/opt/schedule-send __fire 20261004-090000-wBp3
`},
		{"receipt", twoLines, `id=20261003-163000-wBp3
unit=herdr-send-20261003-163000-wBp3.timer
target=agent wB:p3 (codex, idle)
at=2026-10-03 16:30:00
delay_seconds=4080
payload=two

cancel with: schedule-send --cancel 20261003-163000-wBp3
`},
	}
	for _, c := range cases {
		var b strings.Builder
		c.s.Plain(&b)
		if b.String() != c.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, b.String(), c.want)
		}
	}
}

func TestScheduledStyled(t *testing.T) {
	warsaw(t)
	t.Setenv("HOME", "/home/dev")
	long := twoLines
	long.Message = "Carry on with the plan, then commit the migration when it is done\nsecond\nthird\n"
	dry := resume
	dry.DryRun = true

	cases := []struct {
		name string
		s    Scheduled
		want string
	}{
		{"receipt", resume, `
  ✓ scheduled "resume" for claude wB:p1
    at       Sat 16:01   in 39m
    cancel   schedule-send --cancel 20261003-160100-wBp1
`},
		{"receipt, two lines", twoLines, `
  ✓ scheduled "two" (+1 line) for codex wB:p3
    at       Sat 16:30   in 1h 8m
    cancel   schedule-send --cancel 20261003-163000-wBp3
`},
		{"receipt, a long first line and a final newline", long, `
  ✓ scheduled "Carry on with the plan, then commit the…" (+2 lines) for codex wB:p3
    at       Sat 16:30   in 1h 8m
    cancel   schedule-send --cancel 20261003-163000-wBp3
`},
		{"receipt, keys to a plain pane", paneKeys, `
  ✓ scheduled keys: ctrl+c for pane wB:p5
    at       Sun 09:00   in 17h 38m
    cancel   schedule-send --cancel 20261004-090000-wBp5
`},
		{"dry run", dry, `
  · dry run: would schedule "resume" for claude wB:p1
    at       Sat 16:01   in 39m
    cwd      ~/projects/shop-backend
    if-busy  wait
    recover  keys
    grace    4s
    persist  no
    id       20261003-160100-wBp1
    units    /home/dev/.config/systemd/user/herdr-send-20261003-160100-wBp1.{service,timer}
    runs     /opt/schedule-send __fire 20261003-160100-wBp1
`},
		{"dry run, keys, persistent", enterOnly, `
  · dry run: would schedule keys: enter for codex wB:p3
    at       Sun 09:00   in 17h 38m
    cwd      ~/projects/shop-backend
    if-busy  wait
    recover  keys
    grace    4s
    persist  yes
    id       20261004-090000-wBp3
    units    /home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp3.{service,timer}
    runs     /opt/schedule-send __fire 20261004-090000-wBp3
`},
	}
	for _, c := range cases {
		var b strings.Builder
		c.s.Styled(&b, term.Colors{}, at1522)
		if b.String() != c.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, b.String(), c.want)
		}
	}
}

type reportCase struct {
	name string
	r    Report
}

// reportCases is a report for each branch.
func reportCases() []reportCase {
	quote := "resets 4pm (Europe/Warsaw)"
	weekly := limit.Status{State: limit.Limited, Window: "weekly", Reset: at1600, Quote: quote, Source: "wB:p1"}
	unconfirmed := weekly
	unconfirmed.Reset, unconfirmed.Unconfirmed = atSun1600, true
	account := weekly
	account.State = limit.Account
	over := weekly
	over.State = limit.Over
	report := func(st limit.Status, now int64) Report {
		return Report{
			Kind: "agent", Agent: "claude", Pane: "wB:p1", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
			Summary: "claude in wB:p1 (idle, ~/projects/shop-backend)",
			Limit:   st, Message: "resume", Margin: 60, Now: now,
		}
	}
	with := func(r Report, edit func(*Report)) Report { edit(&r); return r }

	return []reportCase{
		{"own", report(weekly, at1522)},
		{"account", with(report(account, at1522), func(r *Report) {
			r.Pane, r.Cwd, r.Summary = "wA:p1", "/home/dev/projects/analytics", "claude in wA:p1 (idle, ~/projects/analytics)"
		})},
		{"over with a reset", report(over, at161640)},
		{"over without one", report(limit.Status{State: limit.Over, Window: "session", Quote: "You've hit your session limit", Source: "wB:p1"}, at161640)},
		{"a reset, for keys", with(report(weekly, at1522), func(r *Report) { r.Message, r.Keys = "", "esc" })},
		{"unconfirmed, on the terminal", report(unconfirmed, at161640)},
		{"unconfirmed, under --auto", with(report(unconfirmed, at161640), func(r *Report) { r.Auto = true })},
		{"no readable reset", with(report(limit.Status{State: limit.Limited, Quote: "You've hit your limit · resets soon", Source: "wB:p1"}, at1522),
			func(r *Report) { r.Message = "carry on\nwith the plan" })},
		{"none", Report{
			Kind: "agent", Agent: "codex", Pane: "wB:p3", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
			Summary: "codex in wB:p3 (idle, ~/projects/shop-backend)",
			Message: "resume", Margin: 60, Now: at1522,
		}},
		{"none, a plain pane", Report{
			Kind: "pane", Agent: "-", Pane: "wB:p5", Status: "-", Cwd: "/home/dev/projects/shop-backend",
			Summary: "pane wB:p5 (~/projects/shop-backend)",
			Message: "ls", Margin: 60, Now: at1522,
		}},
	}
}

func TestReportPlain(t *testing.T) {
	warsaw(t)
	want := map[string]string{
		"own": "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending \"resume\" 1 min after.",
		"account": "Usage limit in claude wB:p1 (per account; wA:p1 shows none):\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending \"resume\" 1 min after.",
		"over with a reset": "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"That reset passed at Sat 16:00; sending \"resume\" 1 min from now.",
		"over without one": "Usage limit in claude wB:p1:\n  You've hit your session limit\n" +
			"Its footer shows usage left again; sending \"resume\" 1 min from now.",
		"a reset, for keys": "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending keys: esc 1 min after.",
		"unconfirmed, on the terminal": "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sun 16:00 (in 23h 44m); sending \"resume\" 1 min after.\n" +
			"The notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min.",
		"unconfirmed, under --auto": "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sun 16:00 (in 23h 44m); sending \"resume\" 1 min after.\n" +
			"The notice names no day; if it was shown before Sat 16:00, the limit is already over: rerun with --at '+1 min'.",
		"no readable reset": "Usage limit in claude wB:p1:\n  You've hit your limit · resets soon\n" +
			"It names no reset time that can be read; sending \"carry on\" at the time given.",
		"none":               "No usage-limit notice found for codex in wB:p3 (idle, ~/projects/shop-backend).",
		"none, a plain pane": "No usage-limit notice found for pane wB:p5 (~/projects/shop-backend).",
	}
	for _, c := range reportCases() {
		w, ok := want[c.name]
		if !ok {
			t.Fatalf("no text for %s", c.name)
		}
		if got := c.r.Plain(); got != w {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, got, w)
		}
	}
}

func TestReportStyled(t *testing.T) {
	warsaw(t)
	want := map[string]string{
		"own": `  ◷ claude wB:p1 hit its weekly limit
    resets   Sat 16:00   in 38m
    send     "resume"    1 min after`,
		"account": `  ◷ claude wB:p1 hit its weekly limit (per account; wA:p1 shows none)
    resets   Sat 16:00   in 38m
    send     "resume"    1 min after`,
		"over with a reset": `  ✓ claude wB:p1's weekly limit is over
    reset    Sat 16:00   passed
    send     "resume"    1 min from now`,
		"over without one": `  ✓ claude wB:p1's session limit is over
    footer   shows usage left again
    send     "resume"    1 min from now`,
		"a reset, for keys": `  ◷ claude wB:p1 hit its weekly limit
    resets   Sat 16:00   in 38m
    send     keys: esc   1 min after`,
		"unconfirmed, on the terminal": `  ◷ claude wB:p1 hit its weekly limit
    resets   Sun 16:00   in 23h 44m
    send     "resume"    1 min after
    note     the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min`,
		"unconfirmed, under --auto": `  ◷ claude wB:p1 hit its weekly limit
    resets   Sun 16:00   in 23h 44m
    send     "resume"    1 min after
    note     the notice names no day; if it was shown before Sat 16:00, the limit is already over: rerun with --at '+1 min'`,
		"no readable reset": `  ◷ claude wB:p1 hit a usage limit
    notice   "You've hit your limit · resets soon"
    send     "carry on" (+1 line) at the time given`,
		"none":               `  · no usage-limit notice for codex wB:p3`,
		"none, a plain pane": `  · no usage-limit notice for pane wB:p5`,
	}
	for _, c := range reportCases() {
		w, ok := want[c.name]
		if !ok {
			t.Fatalf("no text for %s", c.name)
		}
		if got := c.r.Styled(term.Colors{}); got != w {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, got, w)
		}
	}
}

// Sends stored on disk, listed at 15:22: one armed, one due, one whose timer
// is gone, and one whose time cannot be read.
var stored = []Job{
	{ID: "20261003-160100-wBp1", At: "2026-10-03 16:01:00", RunAt: 1791036060, Kind: "agent", Agent: "claude", Pane: "wB:p1",
		State: "active", Payload: "resume", Message: "resume"},
	{ID: "20261003-150000-wAp1", At: "2026-10-03 15:00:00", RunAt: 1791032400, Kind: "agent", Agent: "claude", Pane: "wA:p1",
		State: "active", Payload: "carry on", Message: "carry on\nthen commit\nthen push\n"},
	{ID: "20261004-090000-wBp5", At: "2026-10-04 09:00:00", RunAt: 1791097200, Kind: "pane", Agent: "-", Pane: "wB:p5",
		State: "gone", Payload: "keys: ctrl+c", Keys: "ctrl+c"},
	{ID: "broken", At: "soon", Kind: "agent", Agent: "codex", Pane: "wB:p3", State: "active", Payload: "x", Message: "x"},
}

func TestList(t *testing.T) {
	var b strings.Builder
	List(&b, nil)
	if b.String() != "No scheduled sends.\n" {
		t.Errorf("empty: %q", b.String())
	}

	// tools/parity's list case.
	b.Reset()
	List(&b, []Job{
		{ID: "20261003-160100-wBp1", At: "2026-10-03 16:01:00", RunAt: 1791036060, Pane: "wB:p1", State: "inactive", Payload: "resume"},
		{ID: "20261004-090000-wB5", At: "2026-10-03 16:01:00", RunAt: 1791036060, Pane: "wB:p1", State: "inactive", Payload: "keys: ctrl+c"},
	})
	want := `ID                       AT                   TARGET     STATE     PAYLOAD
20261003-160100-wBp1     2026-10-03 16:01:00  wB:p1      inactive  resume
20261004-090000-wB5      2026-10-03 16:01:00  wB:p1      inactive  keys: ctrl+c
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestListStyled(t *testing.T) {
	warsaw(t)
	var b strings.Builder
	ListStyled(&b, term.Colors{}, nil, at1522)
	if b.String() != "  · no scheduled sends\n" {
		t.Errorf("empty: %q", b.String())
	}

	b.Reset()
	ListStyled(&b, term.Colors{}, stored, at1522)
	want := `
  ○ Sat 16:01   in 39m    claude wB:p1   "resume"                   20261003-160100-wBp1
  ○ Sat 15:00   due       claude wA:p1   "carry on" (+2 lines)      20261003-150000-wAp1
  ✗ Sun 09:00   gone      pane wB:p5     keys: ctrl+c               20261004-090000-wBp5
  ○ soon                  codex wB:p3    "x"                        broken
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestTime(t *testing.T) {
	loc := warsaw(t)
	at := func(mo time.Month, d, hr, mi, s int) int64 { return time.Date(2026, mo, d, hr, mi, s, 0, loc).Unix() }
	cases := []struct {
		epoch int64
		want  string
	}{
		{at1600, "Sat 16:00"},
		{at(10, 3, 0, 5, 0), "Sat 00:05"},
		{at(10, 9, 23, 59, 0), "Fri 23:59"},
		{at(9, 27, 0, 27, 0), "Sun 00:27"},
		{at(10, 10, 0, 27, 0), "Sat Oct 10 00:27"},
		{at(9, 26, 23, 59, 0), "Sat Sep 26 23:59"},
		{at161640, "Sat 16:16:40"},
		{at(10, 10, 0, 27, 52), "Sat Oct 10 00:27:52"},
	}
	for _, c := range cases {
		if got := Time(c.epoch, at1522); got != c.want {
			t.Errorf("Time(%d) = %q, want %q", c.epoch, got, c.want)
		}
	}
}

func TestStyledColoursTheVisibleText(t *testing.T) {
	// Every column is padded by what it shows, so the codes never shift one.
	warsaw(t)
	var b strings.Builder
	for _, c := range reportCases() {
		switch c.name {
		case "account", "over with a reset", "unconfirmed, on the terminal", "none":
			b.WriteString(c.r.Styled(markers) + "\n")
		}
	}
	twoLines.Styled(&b, markers, at1522)
	ListStyled(&b, markers, stored[:3], at1522)
	ListStyled(&b, markers, nil, at1522)

	want := `  <y>◷</> claude wB:p1 hit its weekly limit<dim> (per account; wA:p1 shows none)</>
    <dim>resets</>   Sat 16:00   <dim>in 38m</>
    <dim>send</>     "resume"    <dim>1 min after</>
  <g>✓</> claude wB:p1's weekly limit is over
    <dim>reset</>    Sat 16:00   <dim>passed</>
    <dim>send</>     "resume"    <dim>1 min from now</>
  <y>◷</> claude wB:p1 hit its weekly limit
    <dim>resets</>   Sun 16:00   <dim>in 23h 44m</>
    <dim>send</>     "resume"    <dim>1 min after</>
    <dim>note</>     <y>the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min</>
  <dim>·</> no usage-limit notice for codex wB:p3

  <g>✓</> scheduled "two"<dim> (+1 line)</> for codex wB:p3
    <dim>at</>       Sat 16:30   <dim>in 1h 8m</>
    <dim>cancel</>   schedule-send --cancel 20261003-163000-wBp3

  ○ Sat 16:01   <dim>in 39m</>    claude wB:p1   "resume"                   <dim>20261003-160100-wBp1</>
  ○ Sat 15:00   <dim>due</>       claude wA:p1   "carry on"<dim> (+2 lines)</>      <dim>20261003-150000-wAp1</>
  <r>✗</> Sun 09:00   <r>gone</>      pane wB:p5     keys: ctrl+c               <dim>20261004-090000-wBp5</>
  <dim>·</> no scheduled sends
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}
