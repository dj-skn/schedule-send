package show

import (
	"strings"
	"testing"
	"time"

	"schedule-send/internal/limit"
	"schedule-send/internal/when"
)

func warsaw(t *testing.T) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	prev := when.Loc
	when.Loc = loc
	t.Cleanup(func() { when.Loc = prev })
}

// Sat 2026-10-03 and Sun 2026-10-04, Europe/Warsaw.
const (
	at1522    = 1791033720 // 15:22:00
	at1600    = 1791036000 // 16:00:00
	at161640  = 1791037000 // 16:16:40
	atSun1600 = 1791122400 // Sun 16:00:00
)

func TestScheduledPlain(t *testing.T) {
	// tools/parity's enter-only and arm-text cases.
	cases := []struct {
		name string
		s    Scheduled
		want string
	}{
		{"dry run", Scheduled{
			DryRun: true, ID: "20261004-090000-wBp3", Unit: "herdr-send-20261004-090000-wBp3",
			Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
			At: "2026-10-04 09:00:00", RunAt: 1791097200, Delay: 63480,
			Mode: "keys", Payload: "keys: enter", Keys: "enter",
			Persistent: true, IfBusy: "wait", Recover: "keys", Grace: "4",
			UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp3.{service,timer}",
			WouldRun:  "/opt/schedule-send __fire 20261004-090000-wBp3",
		}, `target=agent wB:p3 (codex, idle)
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
		{"receipt", Scheduled{
			ID: "20261003-163000-wBp3", Unit: "herdr-send-20261003-163000-wBp3",
			Kind: "agent", Pane: "wB:p3", Agent: "codex", Status: "idle", Cwd: "/home/dev/projects/shop-backend",
			At: "2026-10-03 16:30:00", RunAt: 1791037800, Delay: 4080,
			Mode: "text", Payload: "two", Message: "two\nlines",
			IfBusy: "wait", Recover: "keys", Grace: "4",
			UnitFiles: "/home/dev/.config/systemd/user/herdr-send-20261003-163000-wBp3.{service,timer}",
			WouldRun:  "/opt/schedule-send __fire 20261003-163000-wBp3",
		}, `id=20261003-163000-wBp3
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

func TestReportPlain(t *testing.T) {
	warsaw(t)
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
			Agent: "claude", Pane: "wB:p1", Summary: "claude in wB:p1 (idle, ~/projects/shop-backend)",
			Limit: st, What: `"resume"`, Margin: 60, Now: now,
		}
	}

	cases := []struct {
		name string
		r    Report
		want string
	}{
		{"own", report(weekly, at1522), "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending \"resume\" 1 min after."},
		{"account", func() Report {
			r := report(account, at1522)
			r.Pane, r.Summary = "wA:p1", "claude in wA:p1 (idle, ~/projects/analytics)"
			return r
		}(), "Usage limit in claude wB:p1 (per account; wA:p1 shows none):\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending \"resume\" 1 min after."},
		{"over with a reset", report(over, at161640), "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"That reset passed at Sat 16:00; sending \"resume\" 1 min from now."},
		{"over without one", report(limit.Status{State: limit.Over, Window: "session", Quote: "You've hit your session limit", Source: "wB:p1"}, at161640),
			"Usage limit in claude wB:p1:\n  You've hit your session limit\n" +
				"Its footer shows usage left again; sending \"resume\" 1 min from now."},
		{"a reset, for keys", func() Report {
			r := report(weekly, at1522)
			r.What = "keys: esc"
			return r
		}(), "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sat 16:00 (in 38m); sending keys: esc 1 min after."},
		{"unconfirmed, on the terminal", report(unconfirmed, at161640), "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sun 16:00 (in 23h 44m); sending \"resume\" 1 min after.\n" +
			"The notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min."},
		{"unconfirmed, under --auto", func() Report {
			r := report(unconfirmed, at161640)
			r.Auto = true
			return r
		}(), "Usage limit in claude wB:p1:\n  resets 4pm (Europe/Warsaw)\n" +
			"Resets Sun 16:00 (in 23h 44m); sending \"resume\" 1 min after.\n" +
			"The notice names no day; if it was shown before Sat 16:00, the limit is already over: rerun with --at '+1 min'."},
		{"no readable reset", func() Report {
			r := report(limit.Status{State: limit.Limited, Quote: "You've hit your limit · resets soon", Source: "wB:p1"}, at1522)
			r.What = `"carry on"`
			return r
		}(), "Usage limit in claude wB:p1:\n  You've hit your limit · resets soon\n" +
			"It names no reset time that can be read; sending \"carry on\" at the time given."},
		{"none", Report{
			Agent: "codex", Pane: "wB:p3", Summary: "codex in wB:p3 (idle, ~/projects/shop-backend)",
			What: `"resume"`, Margin: 60, Now: at1522,
		}, "No usage-limit notice found for codex in wB:p3 (idle, ~/projects/shop-backend)."},
	}
	for _, c := range cases {
		if got := c.r.Plain(); got != c.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
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
