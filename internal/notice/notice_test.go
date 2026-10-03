package notice

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) Transcript {
	t.Helper()
	b, err := os.ReadFile("../../testdata/fixtures/2026-10-03/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return NewTranscript(string(b))
}

func transcript(lines ...string) Transcript {
	return NewTranscript(strings.Join(lines, "\n") + "\n")
}

func TestCurrentClaudeWeeklyLimit(t *testing.T) {
	n, ok := Current(fixture(t, "visible-wB-p1.txt"))
	if !ok {
		t.Fatal("no notice found on the limited Claude screen")
	}
	if n.Time != "4pm (Europe/Warsaw)" || n.Quote != "resets 4pm (Europe/Warsaw)" || n.Window != "weekly" {
		t.Errorf("got %+v", n)
	}
}

func TestCurrentNoneOnUnlimitedScreens(t *testing.T) {
	for _, name := range []string{"visible-wA-p1.txt", "visible-wB-p3.txt"} {
		if n, ok := Current(fixture(t, name)); ok {
			t.Errorf("%s: unexpected notice %+v", name, n)
		}
	}
}

func TestCurrentCodexWrappedDate(t *testing.T) {
	tr := transcript(
		"■ You’ve hit your usage limit. Upgrade to Pro (https://openai.com/chatgpt/pricing), visit",
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 27th, 2026",
		"12:27 AM.",
		"",
		"                         Tip: something centred",
		"› ",
	)
	n, ok := Current(tr)
	if !ok || n.Time != "Sep 27th, 2026 12:27 AM" || n.Quote != "try again at Sep 27th, 2026 12:27 AM" || n.Window != "" {
		t.Errorf("got %+v ok=%v", n, ok)
	}
}

func TestCurrentCodexResumed(t *testing.T) {
	// Codex echoes a sent prompt with "›" and draws its input box with "»",
	// so one resume puts two prompt lines below the notice.
	limit := "■ You’ve hit your usage limit. Upgrade to Pro (https://openai.com/chatgpt/pricing), visit " +
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 27th, 2026 12:27 AM."
	if n, ok := Current(transcript(limit, "", "› resume", "", "» Ask Codex to do anything")); ok {
		t.Errorf("resumed: got %+v, want the limit treated as over", n)
	}
	if _, ok := Current(transcript(limit, "", "» Ask Codex to do anything")); !ok {
		t.Error("not resumed: no notice found")
	}
}

func TestCurrentStopsAtIndentedTip(t *testing.T) {
	tr := transcript(
		"  ■ Usage limit reached",
		"                 Tip: try again later",
		"› ",
	)
	n, ok := Current(tr)
	if !ok || n.Time != "" || n.Quote != " ■ Usage limit reached" {
		t.Errorf("got %+v ok=%v", n, ok)
	}
}

func TestCurrentClaudeDatedForms(t *testing.T) {
	for _, c := range []struct{ line, time, window string }{
		{"You've hit your session limit · resets 3pm", "3pm", "session"},
		{"You've hit your session limit · resets 3:30pm (Europe/Warsaw)", "3:30pm (Europe/Warsaw)", "session"},
		{"You've hit your weekly limit · resets Sep 28, 3pm", "Sep 28, 3pm", "weekly"},
	} {
		n, ok := Current(transcript(c.line, "❯ "))
		if !ok || n.Time != c.time || n.Window != c.window {
			t.Errorf("%q: got %+v ok=%v, want time %q window %q", c.line, n, ok, c.time, c.window)
		}
	}
}

func TestCurrentMovedPast(t *testing.T) {
	// A prompt went through below the notice without drawing another one.
	tr := transcript(
		"You've hit your session limit · resets 3pm",
		"❯ resume",
		"● Resuming.",
		"❯ ",
	)
	if n, ok := Current(tr); ok {
		t.Errorf("got %+v, want the limit treated as over", n)
	}
}

func TestCurrentNewestWins(t *testing.T) {
	tr := transcript(
		"You've hit your session limit · resets 3pm",
		"❯ resume",
		"You've hit your session limit · resets 8pm",
		"❯ ",
	)
	n, ok := Current(tr)
	if !ok || n.Time != "8pm" {
		t.Errorf("got %+v ok=%v", n, ok)
	}
}

func TestAfter(t *testing.T) {
	old := "You've hit your session limit · resets 3pm"
	cases := []struct {
		name   string
		tr     Transcript
		anchor string
		found  bool
	}{
		{"notice below the echo", transcript("❯ resume", "  ⎿  You've hit your session limit · resets 8pm", "❯ "), "resume", true},
		{"notice only above the echo", transcript(old, "❯ resume", "● Resuming.", "❯ "), "resume", false},
		{"anchor missing", transcript(old, "❯ "), "resume", false},
		{"empty anchor", transcript("❯ resume", old), "", false},
		{"anchor wrapped across lines", transcript("❯ resume, then com", "  mit when done", "■ Usage limit reached"), "resumethencommitwhendone", true},
	}
	for _, c := range cases {
		_, ok := After(c.tr, c.anchor)
		if ok != c.found {
			t.Errorf("%s: found=%v, want %v", c.name, ok, c.found)
		}
	}
}

func TestAfterExcerpt(t *testing.T) {
	tr := transcript("❯ resume", "  ⎿  You've hit your session limit · resets 8pm", "", "❯ ")
	got, _ := After(tr, "resume")
	if got != " ⎿ You've hit your session limit · resets 8pm" {
		t.Errorf("excerpt = %q", got)
	}
	// Without a blank line the next line joins the notice, as it did in the
	// shell version.
	tr = transcript("❯ resume", "  ⎿  You've hit your session limit · resets 8pm", "❯ ")
	if got, _ := After(tr, "resume"); got != " ⎿ You've hit your session limit · resets 8pm ❯ " {
		t.Errorf("joined excerpt = %q", got)
	}
}
