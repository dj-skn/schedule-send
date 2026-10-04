package worker

import (
	"fmt"
	"strings"
	"time"

	"schedule-send/internal/herdr"
	"schedule-send/internal/notice"
	"schedule-send/internal/screen"
)

// Herdr confirms a prompt by observing a lifecycle state change, but that
// observation is not a receipt. Codex has no state hook: herdr scrapes its
// state from the OSC title and its idle rule matches any non-empty title, so
// Codex's state flaps on its own and a prompt whose Enter the TUI dropped still
// satisfies --wait. Herdr records the mirror-image bug for Copilot, where a
// background pane swallowed the keystrokes entirely until prompting was made to
// focus the pane first. Either way the text is left in the composer as an
// unsubmitted draft while the job reports success.
//
// So the send is gated on evidence instead, and the composer outranks the
// lifecycle: a draft still on screen disproves any status the agent claims.
// Only that evidence authorises a keystroke, which is what keeps a retry from
// answering a dialog or landing a second copy of the message.

type verdict int

const (
	unknown verdict = iota
	sent
	unsent
)

// gate is what a submit is judged against: the message's needle, and the
// baseline taken before anything was typed.
type gate struct {
	pane   string
	needle string
	seq0   int64
	// empty is true when the composer was empty before the prompt, which is
	// what lets a non-empty composer afterwards count as our draft.
	empty bool
}

func (w *Worker) composer(pane string) (screen.Composer, error) {
	out, err := w.h.Read(pane, "visible", 40)
	return screen.ParseComposer(screen.Lines(out)), err
}

func (w *Worker) deliver(pane, msg string) error {
	// Baseline taken before anything is typed: the lifecycle counter to compare
	// against, and whether the composer was empty. If it already held a draft
	// of the user's own we cannot tell their text from ours, so the emptiness
	// detector stays off and only the needle speaks.
	g := gate{pane: pane, needle: screen.MessageNeedle(msg)}
	_, g.seq0 = w.h.State(pane)
	c, err := w.composer(pane)
	g.empty = err == nil && c.BodyOK && c.Body == ""
	w.sawDraft = false
	w.recovery = ""

	if g.needle == "" && !g.empty {
		fmt.Fprintf(w.Err, "Note: neither submit detector applies to %s; the send cannot be verified.\n", pane)
	}

	// --wait is deliberately not used. It reports a lifecycle change that a
	// self-flapping agent produces on its own, and when it sees none it fails
	// with agent_prompt_stalled — which is the very case this gate exists to
	// recover from, and treating it as fatal here would make the ladder
	// unreachable. It also holds the job open for the whole turn, so the check
	// would run minutes after the moment it could still fix anything.
	if out, err := w.h.Prompt(pane, msg); err != nil {
		detail := herdr.PromptError(trimNewlines(out))
		if detail == "" {
			detail = err.Error()
		}
		return w.fail("agent prompt to %s was refused: %s", pane, detail)
	}

	note := ""
	switch w.await(g) {
	case sent:
	case unknown:
		if w.sawDraft {
			return w.fail("Could not verify the draft in %s after its screen became unreadable; send aborted.", pane)
		}
		// Nothing in the composer and nothing moved. Pressing Enter here would
		// be the blind keystroke this gate replaced, so the send is reported as
		// delivered but unverified instead of being guessed either way.
		fmt.Fprintf(w.Err, "Delivered to %s but the submit is unverified: no draft in its composer and no state change.\n", pane)
		note = " (unverified)"
	default:
		switch w.recoverSubmission(g) {
		case recovered:
			note = " (" + w.recovery + ")"
		case dialog:
			return w.fail("Agent in %s opened a dialog with the text unsent; it is left in the composer.", pane)
		default:
			return w.fail("Text is still unsent in %s's composer: %s", pane, w.excerpt(pane))
		}
	}

	anchor := screen.LastRunes(screen.Squash(msg), screen.NeedleLen)
	if quote, limited := w.awaitLimitNotice(pane, anchor); limited {
		return w.fail("Agent in %s is still usage-limited; the prompt was not acted on: %s", pane, quote)
	}
	fmt.Fprintf(w.Out, "prompted agent in %s%s\n", pane, note)
	return nil
}

func trimNewlines(s string) string { return strings.TrimRight(s, "\n") }

// verdict is sent, unsent, or unknown, in that order of authority. The composer
// is read first on purpose: whatever the lifecycle says, a draft we can still
// see means the Enter did not land.
func (w *Worker) verdict(g gate) verdict {
	c, err := w.composer(g.pane)
	if err != nil || len(c.Raw) == 0 {
		return unknown
	}
	if g.needle != "" && strings.Contains(c.Tail, g.needle) {
		w.sawDraft = true
		return unsent
	}
	if g.empty && c.BodyOK && c.Body != "" {
		w.sawDraft = true
		return unsent
	}

	// state_change_seq is stamped on an agent when its own status transitions,
	// so comparing it against the baseline is edge-triggered: it still reports
	// a turn that started and finished between two polls, which comparing the
	// status itself would miss.
	status, seq := w.h.State(g.pane)
	if status == "working" || status == "blocked" {
		return sent
	}
	if g.seq0 >= 0 && seq > g.seq0 {
		return sent
	}
	// A draft that was on screen a moment ago and is not there now went
	// somewhere. Only a successfully read, delimited empty composer supports
	// this inference; a changed layout or transformed draft is not proof.
	if w.sawDraft && c.BodyOK && c.Body == "" {
		return sent
	}
	return unknown
}

// await polls until the verdict is decisive or the grace window closes,
// backing off so a genuinely dropped Enter costs a handful of reads rather
// than twenty.
func (w *Worker) await(g gate) verdict {
	deadline := w.Clock.Now().Add(w.grace)
	for polls := 0; ; {
		v := w.verdict(g)
		if v == sent || !w.Clock.Now().Before(deadline) {
			return v
		}
		polls++
		switch {
		case polls < 3:
			w.Clock.Sleep(250 * time.Millisecond)
		case polls < 6:
			w.Clock.Sleep(500 * time.Millisecond)
		default:
			w.Clock.Sleep(time.Second)
		}
	}
}

type outcome int

const (
	exhausted outcome = iota
	recovered
	// dialog means the agent opened one, and the ladder must not touch it.
	dialog
)

// recoverSubmission is the escalation ladder, entered only on a verdict of
// unsent. It presses Enter and never re-sends the text: if the first Enter did
// land after all, a second copy of the message would be far worse than an
// Enter into an empty composer, which every supported TUI ignores.
func (w *Worker) recoverSubmission(g gate) outcome {
	var rungs []string
	switch w.recover {
	case "none":
		return exhausted
	case "focus":
		rungs = []string{"keys", "focus"}
	default:
		rungs = []string{"keys"}
	}

	prevFocus := ""
	for _, rung := range rungs {
		// A previous poll is not permission to press Enter after the screen
		// changes or becomes unreadable. Refresh the draft evidence each time.
		switch w.verdict(g) {
		case sent:
			w.restoreFocus(prevFocus, g.pane)
			return recovered
		case unknown:
			w.restoreFocus(prevFocus, g.pane)
			return exhausted
		}
		if status, _ := w.h.State(g.pane); status == "blocked" {
			w.restoreFocus(prevFocus, g.pane)
			return dialog
		}
		if rung == "focus" {
			// Herdr's own notes record background panes swallowing input until
			// the pane was focused, so focusing is the escalation rather than a
			// louder keypress. It also marks a "done" agent as seen, which reads
			// as idle.
			fmt.Fprintf(w.Err, "Retried Enter did not land in %s; focusing the pane and trying again.\n", g.pane)
			prevFocus = w.h.Focused()
			if err := w.h.Focus(g.pane); err != nil {
				fmt.Fprintf(w.Err, "Could not focus %s: %v\n", g.pane, err)
				continue
			}
			switch w.verdict(g) {
			case sent:
				w.restoreFocus(prevFocus, g.pane)
				return recovered
			case unknown:
				w.restoreFocus(prevFocus, g.pane)
				return exhausted
			}
			w.recovery = "focused, Enter retried"
		} else {
			fmt.Fprintf(w.Err, "Enter was dropped by the agent in %s; retrying the submit.\n", g.pane)
			w.recovery = "Enter retried"
		}

		if w.h.SendKeys("agent", g.pane, []string{"enter"}, nil) != nil {
			fmt.Fprintf(w.Err, "Could not send Enter to %s.\n", g.pane)
			continue
		}
		if w.await(g) == sent {
			w.restoreFocus(prevFocus, g.pane)
			return recovered
		}
	}
	w.restoreFocus(prevFocus, g.pane)
	return exhausted
}

// restoreFocus hands focus back after the focus rung, which is a repair, not a
// context switch. Only an agent can be refocused this way; when a plain pane
// held focus there is nothing to hand it back to.
func (w *Worker) restoreFocus(prev, pane string) {
	if prev == "" || prev == pane {
		return
	}
	_ = w.h.Focus(prev)
}

// excerpt is the draft the user will find in the composer, quoted into the
// failure so the journal and the notification say what is stuck rather than
// only that something is.
func (w *Worker) excerpt(pane string) string {
	out := ""
	c, _ := w.composer(pane)
	for _, l := range c.Raw {
		if !screen.IsBlank(l) {
			out += " " + l
		}
	}
	return screen.Truncate(screen.Collapse(out), 160)
}

// A usage-limited agent takes a prompt like any other: the text leaves the
// composer and the lifecycle moves, so the gate reports a submit. The agent
// then answers with a limit notice instead of a turn, and the job would report
// a success that never happened. So after the submit the transcript below the
// message is watched for that notice.

// awaitLimitNotice holds the job open for limitWatch after the submit and
// returns the notice's text if one showed up below the message's end.
func (w *Worker) awaitLimitNotice(pane, anchor string) (string, bool) {
	if anchor == "" {
		return "", false
	}
	deadline := w.Clock.Now().Add(limitWatch)
	for w.Clock.Now().Before(deadline) {
		w.Clock.Sleep(time.Second)
		if quote, ok := notice.After(w.transcript(pane), anchor); ok {
			return quote, true
		}
	}
	return "", false
}

// transcript is the pane's recent output. Claude draws on the alternate
// screen, whose rows never reach the host scrollback, and herdr can recover
// that history only from an idle agent, so a recent read can come back empty
// and the visible screen, which still holds the latest exchange, stands in.
func (w *Worker) transcript(pane string) notice.Transcript {
	out, err := w.h.Read(pane, "recent-unwrapped", 80)
	if err != nil || strings.TrimSpace(out) == "" {
		out, err = w.h.Read(pane, "visible", 80)
	}
	if err != nil {
		return notice.NewTranscript("")
	}
	return notice.NewTranscript(out)
}
