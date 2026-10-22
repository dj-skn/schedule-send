// Package show holds what a run reports, built before it is printed: the
// scheduled send, the usage-limit report and the job list. Each has a plain
// rendering, which is what scripts read, a styled one for a terminal, and a
// JSON one for --json.
package show

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dj-skn/schedule-send/internal/limit"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
)

// Scheduled is one send, as armed or as a dry run would arm it.
type Scheduled struct {
	DryRun bool
	ID     string
	Unit   string // the timer's name as the platform's scheduler shows it

	// The target, as target.Row has it.
	Kind, Pane, Agent, Status, Cwd string

	At    string // the fire time, as when.Calendar
	RunAt int64  // At as an epoch
	Delay int64  // seconds from now to RunAt

	Mode    string // "text" or "keys"
	Payload string // the message's first line, or "keys: " and the keys
	Message string // the whole message; empty for keys
	Keys    string

	Persistent bool
	IfBusy     string
	Recover    string
	Grace      string // --submit-grace as typed, which is how it is printed
	UnitFiles  string // "<dir>/<unit>.{service,timer}"
	WouldRun   string // the program and arguments the timer runs; empty when this build cannot arm
}

// Plain writes s as key=value lines: every field for a dry run, and for an
// armed send the receipt, which ends with how to cancel it.
func (s Scheduled) Plain(w io.Writer) {
	tgt := fmt.Sprintf("%s %s (%s, %s)", s.Kind, s.Pane, s.Agent, s.Status)
	if s.DryRun {
		persistent := 0
		if s.Persistent {
			persistent = 1
		}
		fmt.Fprintf(w, "target=%s\n", tgt)
		fmt.Fprintf(w, "cwd=%s\n", s.Cwd)
		fmt.Fprintf(w, "at=%s\n", s.At)
		fmt.Fprintf(w, "run_at_epoch=%d\n", s.RunAt)
		fmt.Fprintf(w, "delay_seconds=%d\n", s.Delay)
		fmt.Fprintf(w, "mode=%s\n", s.Mode)
		fmt.Fprintf(w, "payload=%s\n", s.Payload)
		fmt.Fprintf(w, "id=%s\n", s.ID)
		fmt.Fprintf(w, "unit=%s\n", s.Unit)
		fmt.Fprintf(w, "persistent=%d\n", persistent)
		fmt.Fprintf(w, "if_busy=%s\n", s.IfBusy)
		fmt.Fprintf(w, "recover=%s\n", s.Recover)
		fmt.Fprintf(w, "submit_grace=%s\n", s.Grace)
		fmt.Fprintf(w, "unit_files=%s\n", s.UnitFiles)
		fmt.Fprintf(w, "would_run=%s\n", s.WouldRun)
		return
	}
	fmt.Fprintf(w, "id=%s\n", s.ID)
	fmt.Fprintf(w, "unit=%s\n", s.Unit)
	fmt.Fprintf(w, "target=%s\n", tgt)
	fmt.Fprintf(w, "at=%s\n", s.At)
	fmt.Fprintf(w, "delay_seconds=%d\n", s.Delay)
	fmt.Fprintf(w, "payload=%s\n", s.Payload)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "cancel with: schedule-send --cancel %s\n", s.ID)
}

// Styled writes s as a block for a terminal: what goes where and when, then
// how to cancel it, or for a dry run everything the job would hold. The delay
// is counted from now.
func (s Scheduled) Styled(w io.Writer, c term.Colors, now int64) {
	p := palette(c)
	sends := p.sent(s.Message, s.Keys).s + " for " + who(s.Kind, s.Agent, s.Pane)
	at := p.row("at", plain(Time(s.RunAt, now)), "in "+when.Delay(s.RunAt-now))
	lines := []string{""}
	if s.DryRun {
		persist := "no"
		if s.Persistent {
			persist = "yes"
		}
		lines = append(lines,
			headline(p.dim("·"), "dry run: would schedule "+sends), at,
			p.row("cwd", plain(target.PrettyCwd(s.Cwd)), ""),
			p.row("if-busy", plain(s.IfBusy), ""),
			p.row("recover", plain(s.Recover), ""),
			p.row("grace", plain(s.Grace+"s"), ""),
			p.row("persist", plain(persist), ""),
			p.row("id", plain(s.ID), ""),
			p.row("units", plain(s.UnitFiles), ""))
		// A build that cannot arm jobs has no command to show.
		if s.WouldRun != "" {
			lines = append(lines, p.row("runs", plain(s.WouldRun), ""))
		}
	} else {
		lines = append(lines,
			headline(p.green("✓"), "scheduled "+sends), at,
			p.row("cancel", plain("schedule-send --cancel "+s.ID), ""))
	}
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}

// Report says what was found of the usage limit holding the target, before the
// send time is confirmed on the terminal, or on stderr under --auto.
type Report struct {
	Kind, Agent, Pane string       // the target
	Status, Cwd       string       // the target's, as target.Row has them
	Summary           string       // target.Summary of the target
	Limit             limit.Status // the target's; None for a plain pane
	Message, Keys     string       // what is sent: the keys when set, else the message
	Margin            int64        // seconds from the reset, or from now once it passed, to the send
	Auto              bool         // --auto: there is no prompt to type a time at
	Now               int64        // when the run started
}

// Plain is the report as text, without a final newline.
func (r Report) Plain() string {
	st := r.Limit
	if st.State == limit.None {
		return "No usage-limit notice found for " + r.Summary + "."
	}
	what := `"` + firstLine(r.Message) + `"`
	if r.Keys != "" {
		what = "keys: " + r.Keys
	}
	var b strings.Builder
	if st.State == limit.Account {
		fmt.Fprintf(&b, "Usage limit in %s %s (per account; %s shows none):\n", r.Agent, st.Source, r.Pane)
	} else {
		fmt.Fprintf(&b, "Usage limit in %s %s:\n", r.Agent, r.Pane)
	}
	fmt.Fprintf(&b, "  %s\n", st.Quote)
	mins := r.Margin / 60
	switch {
	case st.State == limit.Over && st.Reset != 0:
		fmt.Fprintf(&b, "That reset passed at %s; sending %s %d min from now.",
			when.Format(st.Reset, when.WeekdayMinute), what, mins)
	case st.State == limit.Over:
		fmt.Fprintf(&b, "Its footer shows usage left again; sending %s %d min from now.", what, mins)
	case st.Reset != 0:
		fmt.Fprintf(&b, "Resets %s (in %s); sending %s %d min after.",
			when.Format(st.Reset, when.WeekdayMinute), when.Delay(st.Reset-r.Now), what, mins)
		if st.Unconfirmed {
			before, advice := r.unconfirmed()
			fmt.Fprintf(&b, "\nThe notice names no day; if it was shown before %s, the limit is already over: %s.",
				when.Format(before, when.WeekdayMinute), advice)
		}
	default:
		fmt.Fprintf(&b, "It names no reset time that can be read; sending %s at the time given.", what)
	}
	return b.String()
}

// Styled is the report as a block for a terminal, without the blank line
// before it or a final newline.
func (r Report) Styled(c term.Colors) string {
	p := palette(c)
	st := r.Limit
	if st.State == limit.None {
		return headline(p.dim("·"), "no usage-limit notice for "+who(r.Kind, r.Agent, r.Pane))
	}
	holder := who(r.Kind, r.Agent, r.Pane)
	if st.State == limit.Account {
		holder = r.Agent + " " + st.Source
	}
	name := "usage limit"
	if st.Window != "" {
		name = st.Window + " limit"
	}
	var lines []string
	switch {
	case st.State == limit.Over:
		lines = append(lines, headline(p.green("✓"), holder+"'s "+name+" is over"))
	case st.Window == "":
		lines = append(lines, headline(p.yellow("◷"), holder+" hit a "+name))
	default:
		lines = append(lines, headline(p.yellow("◷"), holder+" hit its "+name))
	}
	if st.State == limit.Account {
		lines[0] += p.dim(" (per account; " + r.Pane + " shows none)").s
	}

	what, mins := p.sent(r.Message, r.Keys), r.Margin/60
	switch {
	case st.State == limit.Over && st.Reset != 0:
		lines = append(lines,
			p.row("reset", plain(Time(st.Reset, r.Now)), "passed"),
			p.row("send", what, fmt.Sprintf("%d min from now", mins)))
	case st.State == limit.Over:
		lines = append(lines,
			p.row("footer", plain("shows usage left again"), ""),
			p.row("send", what, fmt.Sprintf("%d min from now", mins)))
	case st.Reset != 0:
		lines = append(lines,
			p.row("resets", plain(Time(st.Reset, r.Now)), "in "+when.Delay(st.Reset-r.Now)),
			p.row("send", what, fmt.Sprintf("%d min after", mins)))
		if st.Unconfirmed {
			before, advice := r.unconfirmed()
			lines = append(lines, p.row("note", p.yellow(fmt.Sprintf(
				"the notice names no day; if it was shown before %s, the limit is already over: %s",
				Time(before, r.Now), advice)), ""))
		}
	default:
		lines = append(lines,
			p.row("notice", plain(`"`+st.Quote+`"`), ""),
			p.row("send", what, "at the time given"))
	}
	return strings.Join(lines, "\n")
}

// unconfirmed is, for a reset that may already have passed, the moment before
// which a notice naming it would be yesterday's, and what to do if it was.
func (r Report) unconfirmed() (before int64, advice string) {
	// A calendar day earlier, which is not 24 h across a DST change.
	before = time.Unix(r.Limit.Reset, 0).In(when.Loc).AddDate(0, 0, -1).Unix()
	advice = "type +1 min"
	if r.Auto {
		advice = "rerun with --at '+1 min'"
	}
	return before, advice
}

// Job is one stored send, as --list shows it.
type Job struct {
	ID                string
	At                string // the fire time as stored, when.Calendar in when.Loc
	RunAt             int64  // At as an epoch; 0 when it does not parse
	Kind, Agent, Pane string // the target
	State             string // the timer's systemctl is-active state, "gone", or "invalid"
	Error             string // why an "invalid" job's state could not be used
	Payload           string // the message's preview, or "keys: " and the keys
	Message           string // the whole message; empty for keys
	Keys              string // empty for text
}

// List writes jobs as a table, or says there are none.
func List(w io.Writer, jobs []Job) {
	if len(jobs) == 0 {
		fmt.Fprintln(w, "No scheduled sends.")
		return
	}
	const row = "%-24s %-20s %-10s %-9s %s\n"
	fmt.Fprintf(w, row, "ID", "AT", "TARGET", "STATE", "PAYLOAD")
	for _, j := range jobs {
		fmt.Fprintf(w, row, j.ID, j.At, j.Pane, j.State, j.Payload)
	}
}

// ListStyled writes jobs as rows for a terminal, each saying when it fires and
// how soon, where, and what it sends. A timer that is not active is marked,
// with its state in place of how soon.
func ListStyled(w io.Writer, c term.Colors, jobs []Job, now int64) {
	p := palette(c)
	if len(jobs) == 0 {
		fmt.Fprintln(w, headline(p.dim("·"), "no scheduled sends"))
		return
	}
	fmt.Fprintln(w)
	for _, j := range jobs {
		glyph, at := plain("○"), j.At
		var soon span
		switch {
		case j.State != "active":
			glyph, soon = p.red("✗"), p.red(j.State)
		case j.RunAt == 0:
		case j.RunAt <= now:
			soon = p.dim("due")
		default:
			soon = p.dim("in " + when.Delay(j.RunAt-now))
		}
		if j.RunAt != 0 {
			at = Time(j.RunAt, now)
		}
		fmt.Fprintln(w, "  "+glyph.s+" "+plain(at).pad(11)+" "+soon.pad(9)+" "+
			plain(who(j.Kind, j.Agent, j.Pane)).pad(14)+" "+p.sent(j.Message, j.Keys).pad(26)+" "+p.dim(j.ID).s)
	}
}

// Time is an epoch as the styled output shows it: the weekday and time within
// six days of now either way, with the date further off, and the seconds only
// when they are not zero.
func Time(epoch, now int64) string {
	day := func(e int64) time.Time {
		y, m, d := time.Unix(e, 0).In(when.Loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	layout := when.WeekdayMinute
	if days := day(epoch).Sub(day(now)) / (24 * time.Hour); days < -6 || days > 6 {
		layout = "Mon Jan 2 15:04"
	}
	if time.Unix(epoch, 0).In(when.Loc).Second() != 0 {
		layout += ":05"
	}
	return when.Format(epoch, layout)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// who names a target: an agent with its pane, or a plain pane.
func who(kind, agent, pane string) string {
	if kind == "agent" {
		return agent + " " + pane
	}
	return "pane " + pane
}

// headline is a block's first line: a glyph, then what it says.
func headline(glyph span, text string) string { return "  " + glyph.s + " " + text }

// span is a piece of styled output: s with its colour codes, and n, how many
// columns it shows as.
type span struct {
	s string
	n int
}

func plain(s string) span { return span{s, utf8.RuneCountInString(s)} }

// pad fills a out to width columns.
func (a span) pad(width int) string { return a.s + strings.Repeat(" ", max(0, width-a.n)) }

// palette paints text in the colours of one output.
type palette term.Colors

func (p palette) paint(code, s string) span {
	if code == "" {
		return plain(s)
	}
	return span{code + s + p.Reset, utf8.RuneCountInString(s)}
}

func (p palette) dim(s string) span    { return p.paint(p.Dim, s) }
func (p palette) yellow(s string) span { return p.paint(p.Yellow, s) }
func (p palette) green(s string) span  { return p.paint(p.Green, s) }
func (p palette) red(s string) span    { return p.paint(p.Red, s) }
func (p palette) cyan(s string) span   { return p.paint(p.Cyan, s) }

// row is a line under a headline: a dim label, then value, then a dim second
// part, lined up with the rows around it, when there is one.
func (p palette) row(label string, value span, second string) string {
	line := "    " + p.dim(label).pad(8) + " "
	if second == "" {
		return line + value.s
	}
	return line + value.pad(11) + " " + p.dim(second).s
}

// sent is what a send delivers: its keys, or a message's first line in quotes,
// cut to 40 runes, with how many lines follow it.
func (p palette) sent(message, keys string) span {
	if keys != "" {
		return plain("keys: " + keys)
	}
	message = strings.TrimSuffix(message, "\n")
	first := firstLine(message)
	if r := []rune(first); len(r) > 40 {
		first = string(r[:39]) + "…"
	}
	out := plain(`"` + first + `"`)
	n := strings.Count(message, "\n")
	if n == 0 {
		return out
	}
	more := fmt.Sprintf(" (+%d lines)", n)
	if n == 1 {
		more = " (+1 line)"
	}
	tail := p.dim(more)
	return span{out.s + tail.s, out.n + tail.n}
}

// JSON writes s as one object. st is the target's usage limit; a None state is
// left out. note is the picker's short description of it.
func (s Scheduled) JSON(w io.Writer, st limit.Status, note string) {
	out := scheduledJSON{
		DryRun: s.DryRun, ID: s.ID, Unit: s.Unit,
		At: s.At, RunAt: s.RunAt, Delay: s.Delay,
		Mode: s.Mode, Payload: s.Payload, Persistent: s.Persistent,
		IfBusy: s.IfBusy, Recover: s.Recover, Grace: s.Grace,
		Limit: limitAsJSON(st, note),
	}
	out.Target.Kind, out.Target.Pane, out.Target.Agent = s.Kind, s.Pane, s.Agent
	out.Target.Status, out.Target.Cwd = s.Status, s.Cwd
	if s.DryRun {
		out.UnitFiles, out.WouldRun = s.UnitFiles, s.WouldRun
	} else {
		out.Cancel = "schedule-send --cancel " + s.ID
	}
	writeJSON(w, out)
}

type scheduledJSON struct {
	DryRun     bool       `json:"dry_run"`
	ID         string     `json:"id"`
	Unit       string     `json:"unit"`
	Target     targetJSON `json:"target"`
	At         string     `json:"at"`
	RunAt      int64      `json:"run_at_epoch"`
	Delay      int64      `json:"delay_seconds"`
	Mode       string     `json:"mode"`
	Payload    string     `json:"payload"`
	Persistent bool       `json:"persistent"`
	IfBusy     string     `json:"if_busy"`
	Recover    string     `json:"recover"`
	Grace      string     `json:"submit_grace"`
	UnitFiles  string     `json:"unit_files,omitempty"`
	WouldRun   string     `json:"would_run,omitempty"`
	Cancel     string     `json:"cancel,omitempty"`
	Limit      *limitJSON `json:"limit,omitempty"`
}

type targetJSON struct {
	Kind   string `json:"kind"`
	Pane   string `json:"pane"`
	Agent  string `json:"agent"`
	Status string `json:"status"`
	Cwd    string `json:"cwd"`
}

// ListJSON writes jobs as {"jobs":[...]}. An empty list is an empty array.
func ListJSON(w io.Writer, jobs []Job) {
	rows := make([]jobJSON, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, jobJSON{
			ID: j.ID, At: j.At, RunAt: j.RunAt, Kind: j.Kind,
			Agent: j.Agent, Pane: j.Pane, State: j.State, Payload: j.Payload, Error: j.Error,
		})
	}
	writeJSON(w, struct {
		Jobs []jobJSON `json:"jobs"`
	}{rows})
}

type jobJSON struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	RunAt   int64  `json:"run_at_epoch"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent"`
	Pane    string `json:"pane"`
	State   string `json:"state"`
	Payload string `json:"payload"`
	Error   string `json:"error,omitempty"`
}

// Target is one pane as --targets --json prints it.
type Target struct {
	Kind, Pane, Agent, Status, Cwd, Title string
	Limit                                 limit.Status
	Note                                  string
}

// TargetsJSON writes rows as {"targets":[...]}.
func TargetsJSON(w io.Writer, rows []Target) {
	out := make([]listedJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, listedJSON{
			Kind: r.Kind, Pane: r.Pane, Agent: r.Agent, Status: r.Status,
			Cwd: r.Cwd, Title: r.Title, Limit: limitAsJSON(r.Limit, r.Note),
		})
	}
	writeJSON(w, struct {
		Targets []listedJSON `json:"targets"`
	}{out})
}

type listedJSON struct {
	Kind   string     `json:"kind"`
	Pane   string     `json:"pane"`
	Agent  string     `json:"agent"`
	Status string     `json:"status"`
	Cwd    string     `json:"cwd"`
	Title  string     `json:"title"`
	Limit  *limitJSON `json:"limit,omitempty"`
}

// limitJSON is a usage limit as --json prints it.
type limitJSON struct {
	State       string `json:"state"`
	Window      string `json:"window,omitempty"`
	Reset       int64  `json:"reset,omitempty"`
	Unconfirmed bool   `json:"unconfirmed,omitempty"`
	Source      string `json:"source,omitempty"`
	Quote       string `json:"quote,omitempty"`
	Note        string `json:"note,omitempty"`
}

func limitAsJSON(st limit.Status, note string) *limitJSON {
	var name string
	switch st.State {
	case limit.Limited:
		name = "limited"
	case limit.Over:
		name = "over"
	case limit.Account:
		name = "account"
	default:
		return nil
	}
	return &limitJSON{
		State: name, Window: st.Window, Reset: st.Reset, Unconfirmed: st.Unconfirmed,
		Source: st.Source, Quote: st.Quote, Note: note,
	}
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
