// Package target is the list of things a send can go to: Herdr agents and
// plain panes, filtered by the selectors, ranked, and drawn as the picker.
package target

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"schedule-send/internal/term"
)

// Row is one agent or plain pane. Fields herdr left out hold "-", so a row
// reads the same wherever it is shown.
type Row struct {
	Kind   string // "agent" or "pane"
	Pane   string // pane id, e.g. "wB:p1"
	Agent  string // agent kind, e.g. "claude"; "-" for a plain pane
	Status string // idle, done, working, blocked, unknown; "-" for a plain pane
	Cwd    string
	Title  string
}

// IsAgent reports whether the row is an agent rather than a plain pane.
func (r Row) IsAgent() bool { return r.Kind == "agent" }

// Filter holds the selectors that narrow the list.
type Filter struct {
	Kind      string // --kind: only agents of this kind
	PanesOnly bool   // --panes-only
	Cwd       string // --cwd, already normalised with NormPath
	AllowSelf bool   // --allow-self
	Self      string // HERDR_PANE_ID, the pane this command runs in
}

// Apply returns the rows the selectors keep, in their original order.
func (f Filter) Apply(rows []Row) []Row {
	var out []Row
	for _, r := range rows {
		if f.Kind != "" && (!r.IsAgent() || r.Agent != f.Kind) {
			continue
		}
		if f.PanesOnly && r.Kind != "pane" {
			continue
		}
		if f.Cwd != "" && !CwdMatches(NormPath(r.Cwd), f.Cwd) {
			continue
		}
		if !f.AllowSelf && r.Pane == f.Self {
			continue
		}
		out = append(out, r)
	}

	// A cwd selector that hits a real agent should not stay ambiguous just
	// because plain panes share the directory.
	if f.Cwd != "" && !f.PanesOnly {
		var agents []Row
		for _, r := range out {
			if r.IsAgent() {
				agents = append(agents, r)
			}
		}
		if len(agents) > 0 {
			out = agents
		}
	}
	return out
}

// CwdMatches reports whether dir is want or inside it.
func CwdMatches(dir, want string) bool {
	return dir == want || strings.HasPrefix(dir, want+"/")
}

// NormPath expands a leading ~, makes p absolute, resolves symlinks as far as
// the path exists, and drops a trailing slash.
func NormPath(p string) string {
	if home := os.Getenv("HOME"); p == "~" {
		p = home
	} else if rest, ok := strings.CutPrefix(p, "~/"); ok {
		p = home + "/" + rest
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return strings.TrimSuffix(realpath(p), "/")
}

// realpath resolves symlinks in the longest prefix of p that exists and keeps
// the rest as written, as zsh's :A modifier does.
func realpath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	dir, base := filepath.Split(p)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" || dir == p {
		return p
	}
	return filepath.Join(realpath(dir), base)
}

// Rank orders targets best-first: idle and done agents, then unknown, working,
// and blocked ones, then plain panes.
func Rank(r Row) int {
	if !r.IsAgent() {
		return 1
	}
	switch r.Status {
	case "idle", "done":
		return 10
	case "unknown":
		return 6
	case "working":
		return 4
	case "blocked":
		return 2
	default:
		return 3
	}
}

// Sort returns rows best-first. Ties break on pane id.
func Sort(rows []Row) []Row {
	out := append([]Row(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := Rank(out[i]), Rank(out[j])
		if ri != rj {
			return ri > rj
		}
		return out[i].Pane < out[j].Pane
	})
	return out
}

// PrettyCwd shortens the home directory to ~.
func PrettyCwd(p string) string {
	home := os.Getenv("HOME")
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+"/"); ok {
		return "~/" + rest
	}
	return p
}

// Summary names a row in words, so every selection that skips the picker says
// which agent it landed on rather than leaving it to be inferred.
func Summary(r Row) string {
	if r.IsAgent() {
		return fmt.Sprintf("%s in %s (%s, %s)", r.Agent, r.Pane, r.Status, PrettyCwd(r.Cwd))
	}
	return fmt.Sprintf("pane %s (%s)", r.Pane, PrettyCwd(r.Cwd))
}

// Glyph is the status mark drawn before a row.
func Glyph(r Row, c term.Colors) string {
	if !r.IsAgent() {
		return c.Dim + "·" + c.Reset
	}
	switch r.Status {
	case "idle":
		return c.Green + "○" + c.Reset
	case "done":
		return c.Green + "✓" + c.Reset
	case "working":
		return c.Yellow + "●" + c.Reset
	case "blocked":
		return c.Red + "◐" + c.Reset
	default:
		return c.Dim + "?" + c.Reset
	}
}

// Mark is what the picker shows of an agent's usage limit.
type Mark struct {
	Limited bool   // held by its own limit: drawn as "◷ limited"
	Note    string // shown dim after the title, e.g. "resets 16:00 (weekly)"
}

// Render draws the picker: agents and plain panes in separate groups, each row
// numbered by its place in rows. marks, keyed by pane, may be nil. proc names a
// plain pane's foreground process.
func Render(w io.Writer, rows []Row, marks map[string]Mark, c term.Colors, self string, proc func(pane string) string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, c.Bold+"agents"+c.Reset)
	any := false
	for i, r := range rows {
		if !r.IsAgent() {
			continue
		}
		any = true
		m := marks[r.Pane]
		glyph, status := Glyph(r, c), r.Status
		if m.Limited {
			glyph, status = c.Yellow+"◷"+c.Reset, "limited"
		}
		title := r.Title
		if title == "-" {
			title = ""
		}
		text := c.Dim + title + c.Reset
		if m.Note != "" {
			if title != "" {
				text += "  "
			}
			text += c.Dim + m.Note + c.Reset
		}
		here := ""
		if r.Pane == self {
			here = " " + c.Dim + "← this pane" + c.Reset
		}
		fmt.Fprintf(w, "%3d  %s %-8s %-8s %-8s %-26s %s%s\n",
			i+1, glyph, status, r.Agent, r.Pane, PrettyCwd(r.Cwd), text, here)
	}
	if !any {
		fmt.Fprintln(w, "     "+c.Dim+"none"+c.Reset)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, c.Bold+"panes"+c.Reset)
	any = false
	for i, r := range rows {
		if r.Kind != "pane" {
			continue
		}
		any = true
		label := proc(r.Pane)
		if label == "" {
			label = "-"
		}
		fmt.Fprintf(w, "%3d  %s %-8s %-8s %s\n",
			i+1, c.Dim+"·"+c.Reset, label, r.Pane, PrettyCwd(r.Cwd))
	}
	if !any {
		fmt.Fprintln(w, "     "+c.Dim+"none"+c.Reset)
	}
	fmt.Fprintln(w)
}
