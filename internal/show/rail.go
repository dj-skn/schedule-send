package show

import (
	"fmt"
	"strings"

	"github.com/dj-skn/schedule-send/internal/limit"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
)

// Rail draws the questions a run asks on a terminal as guided steps: a line
// down the left, a marker at each question, and an end that says how the run
// came out. Pipes and --json never get it; they keep the plain and JSON forms.
type Rail struct{ p palette }

// NewRail draws in the colours of the terminal the questions are asked on.
func NewRail(c term.Colors) Rail { return Rail{palette(c)} }

// Top is the rail's first line: what the run does, and how to leave it.
func (r Rail) Top(dryRun bool) string {
	title := "Schedule a send"
	if dryRun {
		title += " · dry run"
	}
	return r.p.dim("┌").s + "  " + plain(title).pad(41) + r.p.dim("Ctrl-C cancels").s
}

// Gap is the rail with nothing beside it, between steps.
func (r Rail) Gap() string { return r.p.dim("│").s }

// Line is text beside the rail; an empty one is a gap.
func (r Rail) Line(s string) string {
	if s == "" {
		return r.Gap()
	}
	return r.Gap() + "  " + s
}

// Step is a question.
func (r Rail) Step(question string) string { return r.p.cyan("◆").s + "  " + question }

// Hint says, dim, what an answer can be.
func (r Rail) Hint(s string) string { return r.Line(r.p.dim(s).s) }

// Input is where an answer is typed. It has no newline, so the answer follows
// it on the same line.
func (r Rail) Input() string { return r.Line(r.p.dim("›").s + " ") }

// Resolved shows how a typed time was read, before Enter confirms it.
func (r Rail) Resolved(at, delay string) string {
	return r.Line("→ " + at + r.p.dim(" · in "+delay+" · Enter keeps it").s)
}

// Error is an answer that cannot be used; the question is asked again.
func (r Rail) Error(msg string) string { return r.Line(r.p.red("✗ " + msg).s) }

// Warning is something to know before the send is armed.
func (r Rail) Warning(msg string) string { return r.Line(r.p.yellow(msg).s) }

// Fail ends the rail with why the run stopped.
func (r Rail) Fail(msg string) string { return r.end(r.p.red("✗").s + " " + msg) }

// Cancelled ends the rail of a run stopped with Ctrl-C while it asked, which
// is before anything is written.
func (r Rail) Cancelled() string { return r.end("Cancelled. Nothing was scheduled.") }

func (r Rail) end(s string) string { return r.p.dim("└").s + "  " + s }

// RailLines is the report as it opens the questions: where the send goes,
// then, for an agent, what its screen says of a usage limit. Each line goes
// beside the rail.
func (r Report) RailLines(c term.Colors) []string {
	p := palette(c)
	place := "pane " + r.Pane
	if r.Kind == "agent" {
		place = r.Agent + " · " + r.Pane
	}
	if r.Cwd != "" {
		place += " · " + target.PrettyCwd(r.Cwd)
	}
	lines := []string{place}
	if r.Kind != "agent" {
		return lines
	}
	if r.Status == "blocked" && r.Keys == "" {
		lines = append(lines, p.red("◐").s+" blocked on a prompt right now")
	}

	st := r.Limit
	name := "usage limit"
	if st.Window != "" {
		name = st.Window + " limit"
	}
	switch {
	case st.State == limit.None:
		return append(lines, p.dim("no usage limit on its screen").s)
	case st.State == limit.Over && st.Reset != 0:
		return append(lines, p.green("✓").s+" its "+name+" is over"+
			p.dim(" · the reset at "+Time(st.Reset, r.Now)+" passed").s)
	case st.State == limit.Over:
		return append(lines, p.green("✓").s+" its "+name+" is over"+
			p.dim(" · its footer shows usage left again").s)
	}

	hit := p.yellow("◷").s + " hit its " + name
	if st.Window == "" {
		hit = p.yellow("◷").s + " hit a usage limit"
	}
	if st.Reset != 0 {
		hit += p.dim(" · ").s + "resets " + Time(st.Reset, r.Now) + p.dim(", in "+when.Delay(st.Reset-r.Now)).s
	} else {
		hit += p.dim(" · no reset time could be read").s
	}
	lines = append(lines, hit)
	if st.State == limit.Account {
		lines = append(lines, p.dim("  per account: the notice is in "+st.Source+"; this pane shows none").s)
	}
	if st.Reset == 0 && st.Quote != "" {
		lines = append(lines, p.dim(`  "`+st.Quote+`"`).s)
	}
	if st.Unconfirmed {
		before, advice := r.unconfirmed()
		lines = append(lines, p.yellow(fmt.Sprintf("  the notice names no day; if it was shown before %s, the limit is already over: %s",
			Time(before, r.Now), advice)).s)
	}
	return lines
}

// RailEnd ends the rail with s: what was scheduled and when, then how to
// cancel it, or for a dry run what would be and everything the job would
// hold. The delay is counted from now.
func (s Scheduled) RailEnd(c term.Colors, now int64) string {
	p := palette(c)
	what := p.sent(s.Message, s.Keys).s + " for " + Time(s.RunAt, now) + ", in " + when.Delay(s.RunAt-now)
	r := NewRail(c)
	row := func(label, value string) string { return "   " + p.dim(label).pad(8) + " " + value }
	if !s.DryRun {
		return r.end(p.green("✓").s+" Scheduled "+what) + "\n" +
			row("cancel", "schedule-send --cancel "+s.ID)
	}
	persist := "no"
	if s.Persistent {
		persist = "yes"
	}
	lines := []string{
		r.end(p.dim("·").s + " Dry run: would schedule " + what),
		row("cwd", target.PrettyCwd(s.Cwd)),
		row("if-busy", s.IfBusy),
		row("recover", s.Recover),
		row("grace", s.Grace+"s"),
		row("persist", persist),
		row("id", s.ID),
		row("units", s.UnitFiles),
	}
	// A build that cannot arm jobs has no command to show.
	if s.WouldRun != "" {
		lines = append(lines, row("runs", s.WouldRun))
	}
	return strings.Join(lines, "\n")
}
