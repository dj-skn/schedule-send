package target

import (
	"strings"
	"testing"

	"schedule-send/internal/term"
)

var rows = []Row{
	{"agent", "wA:p1", "claude", "idle", "/home/dev/projects/analytics", "Analytics backend rewrite"},
	{"agent", "wB:p1", "claude", "idle", "/home/dev/projects/shop-backend", "checkout-webhook-migration"},
	{"agent", "wB:p3", "codex", "idle", "/home/dev/projects/shop-backend", "Implement SB-42 phase 3.1 | shop-backend"},
	{"agent", "wE:p1", "cursor", "working", "/home/dev/src/schedule-send", "Tidy schedule-send UI"},
	{"pane", "wB:p2", "-", "-", "/home/dev/projects/shop-backend", "-"},
	{"pane", "wB:p5", "-", "-", "/home/dev/projects/shop-backend", "-"},
}

func panes(rs []Row) string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.Pane)
	}
	return strings.Join(ids, " ")
}

func TestFilter(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	cases := []struct {
		name string
		f    Filter
		want string
	}{
		{"none, self excluded", Filter{Self: "wB:p2"}, "wA:p1 wB:p1 wB:p3 wE:p1 wB:p5"},
		{"self allowed", Filter{Self: "wB:p2", AllowSelf: true}, "wA:p1 wB:p1 wB:p3 wE:p1 wB:p2 wB:p5"},
		{"kind", Filter{Kind: "claude"}, "wA:p1 wB:p1"},
		{"panes only", Filter{PanesOnly: true}, "wB:p2 wB:p5"},
		{"cwd prefers agents", Filter{Cwd: "/home/dev/projects/shop-backend"}, "wB:p1 wB:p3"},
		{"cwd with panes only", Filter{Cwd: "/home/dev/projects/shop-backend", PanesOnly: true}, "wB:p2 wB:p5"},
		{"cwd is a prefix, not a substring", Filter{Cwd: "/home/dev/projects/back"}, ""},
		{"cwd parent", Filter{Cwd: "/home/dev/src"}, "wE:p1"},
	}
	for _, c := range cases {
		if got := panes(c.f.Apply(rows)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSort(t *testing.T) {
	mixed := []Row{
		{Kind: "pane", Pane: "wA:p9"},
		{Kind: "agent", Pane: "wC:p1", Status: "blocked"},
		{Kind: "agent", Pane: "wB:p1", Status: "working"},
		{Kind: "agent", Pane: "wZ:p1", Status: "done"},
		{Kind: "agent", Pane: "wA:p2", Status: "idle"},
		{Kind: "agent", Pane: "wA:p3", Status: "unknown"},
		{Kind: "agent", Pane: "wA:p4", Status: "weird"},
	}
	if got := panes(Sort(mixed)); got != "wA:p2 wZ:p1 wA:p3 wB:p1 wA:p4 wC:p1 wA:p9" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMatchesShellPicker(t *testing.T) {
	// The picker as the zsh version drew it on 2026-10-03, run from wB:p2.
	t.Setenv("HOME", "/home/dev")
	shown := Sort(Filter{Self: "wB:p2"}.Apply(rows[:3]))
	shown = append(shown, rows[5])
	var b strings.Builder
	Render(&b, shown, term.Colors{}, "wB:p2", func(string) string { return "zsh" })
	want := `
agents
  1  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite
  2  ○ idle     claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration
  3  ○ idle     codex    wB:p3    ~/projects/shop-backend    Implement SB-42 phase 3.1 | shop-backend

panes
  4  · zsh      wB:p5    ~/projects/shop-backend

`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestRenderEmptyGroupsAndSelfMark(t *testing.T) {
	var b strings.Builder
	Render(&b, []Row{{"agent", "wB:p2", "claude", "done", "/x", "-"}}, term.Colors{}, "wB:p2", nil)
	// An empty title still leaves the column's separator before the mark.
	want := "\nagents\n  1  ✓ done     claude   wB:p2    /x" + strings.Repeat(" ", 26) +
		"← this pane\n\npanes\n     none\n\n"
	if b.String() != want {
		t.Errorf("got %q\nwant %q", b.String(), want)
	}
}

func TestSummary(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	if got := Summary(rows[2]); got != "codex in wB:p3 (idle, ~/projects/shop-backend)" {
		t.Errorf("agent: %q", got)
	}
	if got := Summary(rows[5]); got != "pane wB:p5 (~/projects/shop-backend)" {
		t.Errorf("pane: %q", got)
	}
}

func TestNormPath(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	for in, want := range map[string]string{
		"~/projects/shop-backend/":       "/home/dev/projects/shop-backend",
		"~":                              "/home/dev",
		"/no/such/dir/../x/":             "/no/such/x",
		"/home/dev/projects/./analytics": "/home/dev/projects/analytics",
	} {
		if got := NormPath(in); got != want {
			t.Errorf("NormPath(%q) = %q, want %q", in, got, want)
		}
	}
}
