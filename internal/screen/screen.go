// Package screen reads the text herdr returns for a pane: the normalised form
// both submit detectors compare in, the lines that draw a rule or a prompt, and
// the composer region at the bottom of an agent's screen.
package screen

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lines splits command output the way a shell command substitution does:
// trailing newlines are dropped, then there is one element per line.
func Lines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// Squash keeps letters and digits, folded to lower case. A composer wraps the
// text it holds, and wrapping injects padding, newlines, and gutter glyphs into
// the middle of it; reducing both sides to this form makes the comparison
// survive all three. Letters are Unicode letters: the worker runs under
// systemd's C.UTF-8, where the shell version kept "ż" as well.
func Squash(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

var (
	blanks     = strings.NewReplacer(" ", "", "\t", "")
	ruleGlyphs = strings.NewReplacer("─", "", "━", "", "═", "", "╌", "", "-", "", "=", "", "_", "")
)

// IsBlank reports whether s holds nothing but spaces and tabs.
func IsBlank(s string) bool { return blanks.Replace(s) == "" }

// IsRule reports whether line is a horizontal rule: something drawn only with
// rule glyphs, once spaces and tabs are ignored.
func IsRule(line string) bool {
	t := blanks.Replace(line)
	return t != "" && ruleGlyphs.Replace(t) == ""
}

// IsPromptLine reports whether line opens with a prompt marker: an agent's
// composer, or a prompt it echoed into its transcript. One leading box edge is
// allowed for composers drawn inside a frame.
func IsPromptLine(line string) bool {
	t := blanks.Replace(line)
	if rest, ok := strings.CutPrefix(t, "│"); ok {
		t = rest
	} else if rest, ok := strings.CutPrefix(t, "|"); ok {
		t = rest
	}
	return strings.HasPrefix(t, "›") || strings.HasPrefix(t, "❯") || strings.HasPrefix(t, ">")
}

// Collapse turns tabs into spaces and squeezes each run of spaces into one, as
// tr -s ' \t' ' ' does.
func Collapse(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// Truncate cuts s to at most n bytes, backing off to a whole character.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// LastRunes returns the last n characters of s, or all of it when shorter.
func LastRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// Composer is one read of an agent's composer, which the two submit detectors
// need different slices of.
type Composer struct {
	// Tail is the last prompt marker down to the bottom of the screen, squashed:
	// the domain the message needle is searched in.
	Tail string
	// Body is that marker up to the rule that closes the composer, squashed: the
	// domain emptiness is judged in. The cut matters, because below that rule
	// Claude draws a status footer whose counters move on their own and would
	// make every composer look occupied.
	Body string
	// BodyOK is false when no closing rule was found, which switches the
	// emptiness detector off rather than letting it guess.
	BodyOK bool
	// Raw holds the region's lines as drawn, for quoting a stuck draft.
	Raw []string
}

// ParseComposer finds the composer in a visible screen.
func ParseComposer(lines []string) Composer {
	var c Composer
	start := -1
	for i, l := range lines {
		if IsPromptLine(l) {
			start = i
		}
	}
	if start < 0 {
		return c
	}
	c.Tail = Squash(strings.Join(lines[start:], ""))
	c.Raw = lines[start:]
	for i := start + 1; i < len(lines); i++ {
		if IsRule(lines[i]) {
			c.Body = Squash(strings.Join(lines[start:i], ""))
			c.Raw = lines[start:i]
			c.BodyOK = true
			break
		}
	}
	return c
}

// NeedleLen and NeedleMin bound the end of a message looked for in the
// composer: a long prompt scrolls and only its end stays on screen, and too
// short a needle would match noise.
const (
	NeedleLen = 24
	NeedleMin = 8
)

// MessageNeedle is the squashed end of msg to look for in the composer, or ""
// when the message is too short to be trusted.
func MessageNeedle(msg string) string {
	s := Squash(msg)
	if utf8.RuneCountInString(s) < NeedleMin {
		return ""
	}
	return LastRunes(s, NeedleLen)
}
