// Package notice finds an agent's usage-limit notice in its transcript: the
// newest one still in force, with the reset time it names, and one that
// appears below a prompt the job has just sent.
package notice

import (
	"regexp"
	"strings"

	"schedule-send/internal/screen"
)

// Transcript is a pane's recent output, line by line, with each line also
// squashed so notices can be matched without depending on glyphs, curly
// apostrophes, or the locale.
type Transcript struct {
	Lines []string
	Sq    []string
}

// NewTranscript splits out into lines and squashes each.
func NewTranscript(out string) Transcript {
	t := Transcript{Lines: screen.Lines(out)}
	t.Sq = make([]string, len(t.Lines))
	for i, l := range t.Lines {
		t.Sq[i] = screen.Squash(l)
	}
	return t
}

// IsLimitLine reports whether a squashed line opens a usage-limit notice:
// Codex's "■ You’ve hit your usage limit" or "Usage limit reached", Claude's
// "You've hit your session limit" or "weekly limit".
func IsLimitLine(sq string) bool {
	if rest, ok := strings.CutPrefix(sq, "youvehityour"); ok && strings.Contains(rest, "limit") {
		return true
	}
	return strings.HasPrefix(sq, "usagelimitreached")
}

// Text is the notice opening at line i, joined back into one line. Codex wraps
// it itself, even in the middle of a date, so the lines after it are taken too:
// up to the blank line or rule that ends it, or a line indented well past it,
// such as the tip Codex centres underneath.
func (t Transcript) Text(i int) string {
	out := t.Lines[i]
	indent := leadingSpaces(t.Lines[i])
	for j := i + 1; j <= i+5 && j < len(t.Lines); j++ {
		if screen.IsBlank(t.Lines[j]) || screen.IsRule(t.Lines[j]) {
			break
		}
		if leadingSpaces(t.Lines[j]) >= indent+8 {
			break
		}
		out += " " + t.Lines[j]
	}
	return screen.Collapse(out)
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// Notice is a usage-limit notice as found on screen.
type Notice struct {
	// Time is the reset time as printed, e.g. "Sep 27th, 2026 12:27 AM" or
	// "4pm (Europe/Warsaw)", or "" when the notice names none.
	Time string
	// Quote is the phrase to show back.
	Quote string
	// Window is the limit's window, "session" or "weekly", or "" when the
	// notice does not say, as Codex's "usage limit" does not.
	Window string
}

// resetPhrase matches Codex's "try again at Sep 27th, 2026 12:27 AM" and
// Claude's "resets 3pm" or "resets Sep 28, 3:30pm", either optionally followed
// by "(Europe/Warsaw)". POSIX leftmost-longest matching, as the shell's =~ did.
var resetPhrase = regexp.MustCompilePOSIX(`([Tt]ry again at|resets at|resets) (([A-Z][a-z]{2} [0-9]{1,2}(st|nd|rd|th)?,? ([0-9]{4},? )?)?[0-9]{1,2}(:[0-9]{2})? ?[AaPp][Mm]( \([A-Za-z_]+(/[A-Za-z0-9_+-]+)*\))?)`)

// Current returns the transcript's newest usage-limit notice. ok is false when
// there is none, or when the agent has since moved past it: a prompt echoed
// below the notice went through without drawing another. The last prompt line
// is the composer itself, so it takes two.
func Current(t Transcript) (n Notice, ok bool) {
	last := -1
	for i, s := range t.Sq {
		if IsLimitLine(s) {
			last = i
		}
	}
	if last < 0 {
		return Notice{}, false
	}
	prompts := 0
	for i := last + 1; i < len(t.Lines); i++ {
		if screen.IsPromptLine(t.Lines[i]) {
			prompts++
		}
	}
	if prompts >= 2 {
		return Notice{}, false
	}
	text := t.Text(last)
	n = Notice{Quote: screen.Truncate(text, 160), Window: window(text)}
	if m := resetPhrase.FindStringSubmatch(text); m != nil {
		n.Time, n.Quote = m[2], m[0]
	}
	return n, true
}

func window(text string) string {
	sq := screen.Squash(text)
	switch {
	case strings.Contains(sq, "sessionlimit"):
		return "session"
	case strings.Contains(sq, "weeklylimit"):
		return "weekly"
	}
	return ""
}

// After looks for a notice that opens below the last copy of anchor, the
// squashed end of a message the job just sent. Both Codex and Claude echo a
// submitted prompt, so that copy marks where this send's answer begins, and a
// notice from an earlier limit sits above it. A short anchor can only land
// later in the transcript than the message, which misses a notice but never
// invents one. It returns the notice's text, cut to 200 bytes.
func After(t Transcript, anchor string) (string, bool) {
	if anchor == "" {
		return "", false
	}
	all := strings.Join(t.Sq, "")
	at := strings.LastIndex(all, anchor)
	if at < 0 {
		return "", false
	}
	end := at + len(anchor)

	// The line holding the anchor's last character.
	i, acc := 0, 0
	for ; i < len(t.Sq); i++ {
		acc += len(t.Sq[i])
		if acc >= end {
			break
		}
	}
	for j := i + 1; j < len(t.Sq); j++ {
		if IsLimitLine(t.Sq[j]) {
			return screen.Truncate(t.Text(j), 200), true
		}
	}
	return "", false
}
