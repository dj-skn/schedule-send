package show

import (
	"strings"
	"testing"

	"github.com/dj-skn/schedule-send/internal/term"
)

func TestRailPieces(t *testing.T) {
	r := NewRail(term.Colors{})
	for _, c := range []struct{ got, want string }{
		{r.Top(false), "┌  Schedule a send" + strings.Repeat(" ", 26) + "Ctrl-C cancels"},
		{r.Top(true), "┌  Schedule a send · dry run" + strings.Repeat(" ", 16) + "Ctrl-C cancels"},
		{r.Gap(), "│"},
		{r.Line(""), "│"},
		{r.Line("claude · wB:p1"), "│  claude · wB:p1"},
		{r.Step("When?"), "◆  When?"},
		{r.Hint(`Enter keeps "resume"`), `│  Enter keeps "resume"`},
		{r.Input(), "│  › "},
		{r.Resolved("Sat 16:20", "2m"), "│  → Sat 16:20 · in 2m · Enter keeps it"},
		{r.Error("Sat 16:18 has already passed."), "│  ✗ Sat 16:18 has already passed."},
		{r.Warning("Warning: careful."), "│  Warning: careful."},
		{r.Fail("No send time given."), "└  ✗ No send time given."},
		{r.Cancelled(), "└  Cancelled. Nothing was scheduled."},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestReportRailLines(t *testing.T) {
	warsaw(t)
	t.Setenv("HOME", "/home/dev")
	want := map[string]string{
		"own": `claude · wB:p1 · ~/projects/shop-backend
◷ hit its weekly limit · resets Sat 16:00, in 38m`,
		"account": `claude · wA:p1 · ~/projects/analytics
◷ hit its weekly limit · resets Sat 16:00, in 38m
  per account: the notice is in wB:p1; this pane shows none`,
		"over with a reset": `claude · wB:p1 · ~/projects/shop-backend
✓ its weekly limit is over · the reset at Sat 16:00 passed`,
		"over without one": `claude · wB:p1 · ~/projects/shop-backend
✓ its session limit is over · its footer shows usage left again`,
		"a reset, for keys": `claude · wB:p1 · ~/projects/shop-backend
◷ hit its weekly limit · resets Sat 16:00, in 38m`,
		"unconfirmed, on the terminal": `claude · wB:p1 · ~/projects/shop-backend
◷ hit its weekly limit · resets Sun 16:00, in 23h 44m
  the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min`,
		"unconfirmed, under --auto": `claude · wB:p1 · ~/projects/shop-backend
◷ hit its weekly limit · resets Sun 16:00, in 23h 44m
  the notice names no day; if it was shown before Sat 16:00, the limit is already over: rerun with --at '+1 min'`,
		"no readable reset": `claude · wB:p1 · ~/projects/shop-backend
◷ hit a usage limit · no reset time could be read
  "You've hit your limit · resets soon"`,
		"none": `codex · wB:p3 · ~/projects/shop-backend
no usage limit on its screen`,
		"none, a plain pane": `pane wB:p5 · ~/projects/shop-backend`,
	}
	for _, c := range reportCases() {
		w, ok := want[c.name]
		if !ok {
			t.Fatalf("no text for %s", c.name)
		}
		if got := strings.Join(c.r.RailLines(term.Colors{}), "\n"); got != w {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, got, w)
		}
	}

	t.Run("blocked", func(t *testing.T) {
		r := reportCases()[0].r
		r.Status = "blocked"
		got := strings.Join(r.RailLines(term.Colors{}), "\n")
		w := `claude · wB:p1 · ~/projects/shop-backend
◐ blocked on a prompt right now
◷ hit its weekly limit · resets Sat 16:00, in 38m`
		if got != w {
			t.Errorf("got:\n%s\nwant:\n%s", got, w)
		}
		// Keys are what answers a prompt, so they need no warning.
		r.Message, r.Keys = "", "esc"
		if got := r.RailLines(term.Colors{}); len(got) != 2 {
			t.Errorf("for keys: %q", got)
		}
	})
}

func TestScheduledRailEnd(t *testing.T) {
	warsaw(t)
	t.Setenv("HOME", "/home/dev")
	want := `└  ✓ Scheduled "two" (+1 line) for Sat 16:30, in 1h 8m
   cancel   schedule-send --cancel 20261003-163000-wBp3`
	if got := twoLines.RailEnd(term.Colors{}, at1522); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	want = `└  · Dry run: would schedule keys: enter for Sun 09:00, in 17h 38m
   cwd      ~/projects/shop-backend
   if-busy  wait
   recover  keys
   grace    4s
   persist  yes
   id       20261004-090000-wBp3
   units    /home/dev/.config/systemd/user/herdr-send-20261004-090000-wBp3.{service,timer}
   runs     /opt/schedule-send __fire 20261004-090000-wBp3`
	if got := enterOnly.RailEnd(term.Colors{}, at1522); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRailColoursTheVisibleText(t *testing.T) {
	warsaw(t)
	t.Setenv("HOME", "/home/dev")
	r := NewRail(markers)
	lines := []string{r.Top(true), r.Gap(), r.Step("When?"), r.Hint("18:30"), r.Input(),
		r.Resolved("Sat 16:20", "2m"), r.Error("no"), r.Warning("careful"), r.Fail("stopped"), r.Cancelled()}
	for _, c := range reportCases() {
		switch c.name {
		case "account", "over with a reset", "unconfirmed, on the terminal", "none":
			lines = append(lines, c.r.RailLines(markers)...)
		}
	}
	lines = append(lines, twoLines.RailEnd(markers, at1522))

	want := `<dim>┌</>  Schedule a send · dry run                <dim>Ctrl-C cancels</>
<dim>│</>
<c>◆</>  When?
<dim>│</>  <dim>18:30</>
<dim>│</>  <dim>›</> ` + `
<dim>│</>  → Sat 16:20<dim> · in 2m · Enter keeps it</>
<dim>│</>  <r>✗ no</>
<dim>│</>  <y>careful</>
<dim>└</>  <r>✗</> stopped
<dim>└</>  Cancelled. Nothing was scheduled.
claude · wA:p1 · ~/projects/analytics
<y>◷</> hit its weekly limit<dim> · </>resets Sat 16:00<dim>, in 38m</>
<dim>  per account: the notice is in wB:p1; this pane shows none</>
claude · wB:p1 · ~/projects/shop-backend
<g>✓</> its weekly limit is over<dim> · the reset at Sat 16:00 passed</>
claude · wB:p1 · ~/projects/shop-backend
<y>◷</> hit its weekly limit<dim> · </>resets Sun 16:00<dim>, in 23h 44m</>
<y>  the notice names no day; if it was shown before Sat 16:00, the limit is already over: type +1 min</>
codex · wB:p3 · ~/projects/shop-backend
<dim>no usage limit on its screen</>
<dim>└</>  <g>✓</> Scheduled "two"<dim> (+1 line)</> for Sat 16:30, in 1h 8m
   <dim>cancel</>   schedule-send --cancel 20261003-163000-wBp3`
	if got := strings.Join(lines, "\n"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
