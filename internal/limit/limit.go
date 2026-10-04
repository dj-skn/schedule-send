// Package limit works out from an agent's visible screen whether it is held by
// a usage limit, and when that lifts.
package limit

import (
	"regexp"
	"strconv"
	"sync"
	"time"

	"schedule-send/internal/notice"
	"schedule-send/internal/screen"
	"schedule-send/internal/when"
)

// Gauge is one usage gauge from an agent's footer.
type Gauge struct {
	Window   string        // "session" or "weekly"
	Left     int           // percent left; negative once over
	ResetIn  time.Duration // time to reset as printed
	HasReset bool          // false when the footer shows no reset time
}

// gaugeToken matches Claude's statusline gauges, "session:28% (03:44)" and
// "week:0% (44m)", and Codex's "5h 100% left" and "weekly 3% left". A gauge
// Codex cut off at the pane's edge ("weekl…") does not match.
var gaugeToken = regexp.MustCompile(`(?:^|\s)(?:(session|week):(-?[0-9]+)%(?: \(([0-9dhm:]+)\))?|(5h|weekly) (-?[0-9]+)% left)`)

type resetForm struct {
	re    *regexp.Regexp
	units []time.Duration
}

// The statusline prints a session's time to reset as HH:MM, and a week's as
// XdYh, XhYm, or Xm.
var (
	sessionForms = []resetForm{
		{regexp.MustCompile(`^([0-9]+):([0-5][0-9])$`), []time.Duration{time.Hour, time.Minute}},
	}
	weekForms = []resetForm{
		{regexp.MustCompile(`^([0-9]+)d([0-9]+)h$`), []time.Duration{24 * time.Hour, time.Hour}},
		{regexp.MustCompile(`^([0-9]+)h([0-9]+)m$`), []time.Duration{time.Hour, time.Minute}},
		{regexp.MustCompile(`^([0-9]+)m$`), []time.Duration{time.Minute}},
	}
)

// Gauges reads the footer gauges from the lines below the last prompt line
// (screen.IsPromptLine). A screen with no prompt line has no footer to read.
func Gauges(lines []string) []Gauge {
	last := -1
	for i, l := range lines {
		if screen.IsPromptLine(l) {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	var gs []Gauge
	for _, l := range lines[last+1:] {
		for _, m := range gaugeToken.FindAllStringSubmatch(l, -1) {
			g, ok := parseGauge(m)
			if !ok {
				continue
			}
			if _, seen := gauge(gs, g.Window); !seen {
				gs = append(gs, g)
			}
		}
	}
	return gs
}

func parseGauge(m []string) (Gauge, bool) {
	name, left, reset := m[1], m[2], m[3]
	if name == "" {
		name, left = m[4], m[5]
	}
	g := Gauge{Window: "weekly"}
	if name == "session" || name == "5h" {
		g.Window = "session"
	}
	n, err := strconv.Atoi(left)
	if err != nil {
		return Gauge{}, false
	}
	g.Left = n
	if reset != "" {
		g.ResetIn, g.HasReset = resetIn(g.Window, reset)
	}
	return g, true
}

func resetIn(window, s string) (time.Duration, bool) {
	forms := weekForms
	if window == "session" {
		forms = sessionForms
	}
	for _, f := range forms {
		m := f.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		var d time.Duration
		for i, unit := range f.units {
			n, err := strconv.Atoi(m[i+1])
			if err != nil {
				return 0, false
			}
			d += time.Duration(n) * unit
		}
		return d, true
	}
	return 0, false
}

func gauge(gs []Gauge, window string) (Gauge, bool) {
	for _, g := range gs {
		if g.Window == window {
			return g, true
		}
	}
	return Gauge{}, false
}

// State is how a usage limit holds an agent.
type State int

const (
	None    State = iota // not held
	Limited              // held by its own notice
	Over                 // its own notice's reset has passed; still not resumed
	Account              // no notice of its own, but another agent of the same kind is Limited
)

// Status is an agent's limit state.
type Status struct {
	State       State
	Window      string  // the notice's window: "session", "weekly" or ""
	Reset       int64   // Limited/Account: the reset ahead (0 unknown). Over: the reset that passed (0 unknown)
	Unconfirmed bool    // a time-only reset with nothing to bound it, more than 12 h ahead
	Time        string  // the reset as printed (notice.Notice.Time)
	Quote       string  // notice.Notice.Quote
	Source      string  // the pane whose notice this is (set by Scan)
	Gauges      []Gauge // this agent's own footer gauges
}

const (
	// sessionSpan is the furthest a session limit's reset can be: the window
	// is five hours long.
	sessionSpan = 5 * 3600
	// slack covers a footer's time to reset, which is rounded down to the
	// minute.
	slack = 60
	// unconfirmedAfter is how far ahead an unbounded time-only reset can be
	// before it may well be one that already passed.
	unconfirmedAfter = 12 * 3600
)

// Read is an agent's own limit state from its visible screen at now.
//
// A notice stays on screen after its limit lifts, until the agent is
// prompted again. The agent's own gauge showing room in the notice's window
// means it has lifted. A dated reset settles which side of it now is, but
// Claude's "resets 4pm" names no day, so it is taken as the next 4pm unless
// the agent's own footer, or a session window's five hours, says the reset
// cannot be that far away; then it was the last 4pm, and the limit is over.
func Read(out string, now int64) Status {
	t := notice.NewTranscript(out)
	st := Status{Gauges: Gauges(t.Lines)}
	n, ok := notice.Current(t)
	if !ok {
		return st
	}
	st.Window, st.Time, st.Quote = n.Window, n.Time, n.Quote
	g, hasGauge := gauge(st.Gauges, n.Window)

	var r, prev int64
	known := false
	if n.Time != "" {
		span := int64(7 * 24 * 3600)
		if n.Window == "session" {
			span = sessionSpan + slack
		}
		if hasGauge && g.HasReset {
			span = int64(g.ResetIn/time.Second) + slack
		}
		r, known = when.ResolveReset(n.Time, now, span)
	}
	timeOnly := known && when.TimeOnly(n.Time)
	if timeOnly {
		prev, _ = when.Previous(n.Time, now)
	}

	switch {
	case hasGauge && g.Left > 0:
		st.State = Over
		if timeOnly {
			st.Reset = prev
		} else if known && r <= now {
			st.Reset = r
		}
	case !known:
		st.State = Limited
	case !timeOnly:
		st.State, st.Reset = Limited, r
		if r <= now {
			st.State = Over
		}
	default:
		var bound int64
		bounded := true
		switch {
		case hasGauge && g.HasReset:
			bound = now + int64(g.ResetIn/time.Second) + slack
		case n.Window == "session":
			bound = now + sessionSpan + slack
		default:
			bounded = false
		}
		if bounded && r > bound {
			st.State, st.Reset = Over, prev
		} else {
			st.State, st.Reset = Limited, r
			st.Unconfirmed = !bounded && r-now > unconfirmedAfter
		}
	}
	return st
}

// Agent names one agent for Scan.
type Agent struct{ Pane, Kind string } // Kind is the agent name, e.g. "claude"

// Scan reads every agent's screen at once with read (which must be safe for
// concurrent use), never reading self, and returns each agent's Status keyed
// by pane, with Account filled in.
//
// Agents of one kind may share an account, so one held by its own notice
// marks the others as held too, unless an agent's own footer shows room in
// that window, which means a different account.
func Scan(agents []Agent, self string, now int64, read func(pane string) string) map[string]Status {
	own := make([]Status, len(agents))
	var wg sync.WaitGroup
	for i, a := range agents {
		if a.Pane == self {
			continue
		}
		wg.Go(func() {
			own[i] = Read(read(a.Pane), now)
			if own[i].State == Limited || own[i].State == Over {
				own[i].Source = a.Pane
			}
		})
	}
	wg.Wait()

	out := make(map[string]Status, len(agents))
	for i, a := range agents {
		if a.Pane == self {
			continue
		}
		st := own[i]
		if st.State == None {
			if j := holder(agents, own, a.Kind, st.Gauges); j >= 0 {
				gs := st.Gauges
				st = own[j]
				st.State, st.Gauges = Account, gs
			}
		}
		out[a.Pane] = st
	}
	return out
}

// holder is the first-ranked Limited agent of kind whose notice the target
// gauges do not veto, or -1.
// Agents not read, self among them, are None and never qualify.
func holder(agents []Agent, own []Status, kind string, gs []Gauge) int {
	best := -1
	for i, a := range agents {
		if a.Kind != kind || own[i].State != Limited || vetoed(gs, own[i].Window) {
			continue
		}
		if best < 0 || Before(own[i], own[best]) {
			best = i
		}
	}
	return best
}

// vetoed reports whether an agent's own gauges show room in window. A notice
// that names no window is vetoed only when every gauge shown has room.
func vetoed(gs []Gauge, window string) bool {
	if window != "" {
		g, ok := gauge(gs, window)
		return ok && g.Left > 0
	}
	for _, g := range gs {
		if g.Left <= 0 {
			return false
		}
	}
	return len(gs) > 0
}

// Before reports whether a ranks ahead of b among agents held by their own
// notice: Over first, then the soonest known Reset, unknown resets last.
func Before(a, b Status) bool {
	if (a.State == Over) != (b.State == Over) {
		return a.State == Over
	}
	if (a.Reset == 0) != (b.Reset == 0) {
		return a.Reset != 0
	}
	return a.Reset < b.Reset
}
