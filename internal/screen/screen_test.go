package screen

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/fixtures/2026-10-03/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return Lines(string(b))
}

func TestSquash(t *testing.T) {
	cases := map[string]string{
		"Zażółć Gęślą 123 ❯ ─ You’ve": "zażółćgęślą123youve",
		"  ❯ resume, then commit":     "resumethencommit",
		"[Pasted text #1 +40 lines]":  "pastedtext140lines",
		"":                            "",
	}
	for in, want := range cases {
		if got := Squash(in); got != want {
			t.Errorf("Squash(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsRule(t *testing.T) {
	cases := map[string]bool{
		"──────────":      true,
		"  ── ─ \t":       true,
		"═══━━━╌╌":        true,
		"----====____":    true,
		"":                false,
		"   ":             false,
		"── cache-migr ─": false,
		"──────────────── x": false,
	}
	for in, want := range cases {
		if got := IsRule(in); got != want {
			t.Errorf("IsRule(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsPromptLine(t *testing.T) {
	cases := map[string]bool{
		"❯ hello":                    true,
		"  ❯":                        true,
		"│ > foo":                    true,
		"| › bar":                    true,
		"› x":                        true,
		"» Ask Codex to do anything": true,
		"│ »":                        true,
		"||> double edge":            false,
		"hello ❯":                    false,
		"":                           false,
	}
	for in, want := range cases {
		if got := IsPromptLine(in); got != want {
			t.Errorf("IsPromptLine(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCollapseAndTruncate(t *testing.T) {
	if got := Collapse("  a \t\t b   c "); got != " a b c " {
		t.Errorf("Collapse = %q", got)
	}
	if got := Truncate("ab€cd", 3); got != "ab" {
		t.Errorf("Truncate mid-rune = %q, want %q", got, "ab")
	}
	if got := Truncate("abc", 10); got != "abc" {
		t.Errorf("Truncate short = %q", got)
	}
}

func TestMessageNeedle(t *testing.T) {
	cases := map[string]string{
		"resume":                        "",
		"resume, then commit when done": "resumethencommitwhendone",
		"please run the whole verification gate and report back": "icationgateandreportback",
	}
	for in, want := range cases {
		if got := MessageNeedle(in); got != want {
			t.Errorf("MessageNeedle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseComposerClaude(t *testing.T) {
	// The usage-limited Claude pane: an empty composer between two rules, with
	// the status footer below the closing one.
	c := ParseComposer(fixture(t, "visible-wB-p1.txt"))
	if !c.BodyOK || c.Body != "" {
		t.Fatalf("BodyOK=%v Body=%q, want an empty, delimited composer", c.BodyOK, c.Body)
	}
	if len(c.Raw) != 1 || c.Raw[0] != "❯" {
		t.Errorf("Raw = %q", c.Raw)
	}
	if c.Tail == "" {
		t.Error("Tail should include the footer below the composer")
	}
}

func TestParseComposerEarlierPromptsIgnored(t *testing.T) {
	// Echoed prompts above ("❯ /color") must not be taken for the composer.
	c := ParseComposer(fixture(t, "visible-wA-p1.txt"))
	if !c.BodyOK || c.Body != "" {
		t.Fatalf("BodyOK=%v Body=%q", c.BodyOK, c.Body)
	}
}

func TestParseComposerDraft(t *testing.T) {
	lines := []string{
		"● done",
		"────────────",
		"❯ resume, then commit",
		"  when done",
		"────────────",
		"  ctx:16%  cost:$13.78",
	}
	c := ParseComposer(lines)
	if !c.BodyOK || c.Body != "resumethencommitwhendone" {
		t.Errorf("Body = %q (ok=%v)", c.Body, c.BodyOK)
	}
	if c.Tail != "resumethencommitwhendonectx16cost1378" {
		t.Errorf("Tail = %q", c.Tail)
	}
}

func TestParseComposerUndelimited(t *testing.T) {
	c := ParseComposer([]string{"› hello", "  footer"})
	if c.BodyOK || c.Tail != "hellofooter" || len(c.Raw) != 2 {
		t.Errorf("got %+v", c)
	}
}

func TestParseComposerCodexInputBox(t *testing.T) {
	// A prompt Codex echoed as "› resume" sits above its "»" input box, which
	// is the composer. With no rule under it, emptiness cannot be judged.
	lines := append([]string{"› resume", ""}, fixture(t, "visible-wB-p3.txt")...)
	c := ParseComposer(lines)
	if !strings.HasPrefix(c.Tail, "askcodextodoanything") || strings.Contains(c.Tail, "resume") {
		t.Errorf("Tail = %q, want it to start at the input box", c.Tail)
	}
	if c.BodyOK || len(c.Raw) == 0 || c.Raw[0] != "» Ask Codex to do anything" {
		t.Errorf("BodyOK=%v Raw=%q", c.BodyOK, c.Raw)
	}
}
