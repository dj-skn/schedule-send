// Package show holds what a run reports, built before it is printed: the
// scheduled send, the usage-limit report and the job list, each with its plain
// rendering.
package show

import (
	"fmt"
	"io"
	"strings"
	"time"

	"schedule-send/internal/limit"
	"schedule-send/internal/when"
)

// Scheduled is one send, as armed or as a dry run would arm it.
type Scheduled struct {
	DryRun bool
	ID     string
	Unit   string // the timer's and service's name, without the suffix

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
	WouldRun   string // the command the timer runs
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
		fmt.Fprintf(w, "unit=%s.timer\n", s.Unit)
		fmt.Fprintf(w, "persistent=%d\n", persistent)
		fmt.Fprintf(w, "if_busy=%s\n", s.IfBusy)
		fmt.Fprintf(w, "recover=%s\n", s.Recover)
		fmt.Fprintf(w, "submit_grace=%s\n", s.Grace)
		fmt.Fprintf(w, "unit_files=%s\n", s.UnitFiles)
		fmt.Fprintf(w, "would_run=%s\n", s.WouldRun)
		return
	}
	fmt.Fprintf(w, "id=%s\n", s.ID)
	fmt.Fprintf(w, "unit=%s.timer\n", s.Unit)
	fmt.Fprintf(w, "target=%s\n", tgt)
	fmt.Fprintf(w, "at=%s\n", s.At)
	fmt.Fprintf(w, "delay_seconds=%d\n", s.Delay)
	fmt.Fprintf(w, "payload=%s\n", s.Payload)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "cancel with: schedule-send --cancel %s\n", s.ID)
}

// Report says what was found of the usage limit holding the target, before the
// send time is confirmed on the terminal, or on stderr under --auto.
type Report struct {
	Agent, Pane string       // the target
	Summary     string       // target.Summary of the target
	Limit       limit.Status // the target's; None for a plain pane
	What        string       // what is sent: `"resume"`, or "keys: esc"
	Margin      int64        // seconds from the reset, or from now once it passed, to the send
	Auto        bool         // --auto: there is no prompt to type a time at
	Now         int64        // when the run started
}

// Plain is the report as text, without a final newline.
func (r Report) Plain() string {
	st := r.Limit
	if st.State == limit.None {
		return "No usage-limit notice found for " + r.Summary + "."
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
			when.Format(st.Reset, when.WeekdayMinute), r.What, mins)
	case st.State == limit.Over:
		fmt.Fprintf(&b, "Its footer shows usage left again; sending %s %d min from now.", r.What, mins)
	case st.Reset != 0:
		fmt.Fprintf(&b, "Resets %s (in %s); sending %s %d min after.",
			when.Format(st.Reset, when.WeekdayMinute), when.Delay(st.Reset-r.Now), r.What, mins)
		if st.Unconfirmed {
			// A calendar day earlier, which is not 24 h across a DST change.
			before := time.Unix(st.Reset, 0).In(when.Loc).AddDate(0, 0, -1).Unix()
			advice := "type +1 min"
			if r.Auto {
				advice = "rerun with --at '+1 min'"
			}
			fmt.Fprintf(&b, "\nThe notice names no day; if it was shown before %s, the limit is already over: %s.",
				when.Format(before, when.WeekdayMinute), advice)
		}
	default:
		fmt.Fprintf(&b, "It names no reset time that can be read; sending %s at the time given.", r.What)
	}
	return b.String()
}

// Job is one stored send, as --list shows it.
type Job struct {
	ID      string
	At      string // the fire time as stored, when.Calendar in when.Loc
	RunAt   int64  // At as an epoch; 0 when it does not parse
	Pane    string
	State   string // the timer's systemctl is-active state, or "gone"
	Payload string // the message's preview, or "keys: " and the keys
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
