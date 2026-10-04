package limit

import (
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"schedule-send/internal/screen"
	"schedule-send/internal/when"
)

func warsaw(t *testing.T) *time.Location {
	t.Helper()
	t.Setenv("TZ", "Europe/Warsaw")
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	prev := when.Loc
	when.Loc = loc
	t.Cleanup(func() { when.Loc = prev })
	return loc
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// replace is strings.Replace that fails the test when old is not there.
func replace(t *testing.T, s, old, with string) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("%q not found", old)
	}
	return strings.Replace(s, old, with, 1)
}

// Sat 2026-10-03 and Sun 2026-10-04, Europe/Warsaw.
const (
	at1522    = 1791033720 // 15:22:00
	at1600    = 1791036000 // 16:00:00
	at161640  = 1791037000 // 16:16:40
	atSun1600 = 1791122400 // Sun 16:00:00
)

func TestGauges(t *testing.T) {
	const h, m = time.Hour, time.Minute
	cases := []struct {
		name string
		want []Gauge
	}{
		{"2026-10-03/visible-wB-p1.txt", []Gauge{{"session", 28, 3*h + 44*m, true}, {"weekly", 0, 44 * m, true}}},
		{"2026-10-03/visible-wA-p1.txt", []Gauge{{"weekly", 11, h + 58*m, true}}},
		{"2026-10-03/visible-wB-p3.txt", []Gauge{{"session", 100, 0, false}}},
		{"2026-10-03-evening/visible-wB-p1.txt", []Gauge{{"session", -1, h + 11*m, true}, {"weekly", 95, 6*24*h + 22*h, true}}},
	}
	for _, c := range cases {
		if got := Gauges(screen.Lines(fixture(t, c.name))); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestGaugesForms(t *testing.T) {
	const h = time.Hour
	rule := strings.Repeat("─", 40)
	cases := []struct {
		footer string
		want   []Gauge
	}{
		{"GPT-6.1-Sol ultra · 5h 40% left · weekly 3% left", []Gauge{{"session", 40, 0, false}, {"weekly", 3, 0, false}}},
		{"ctx:9%  session:5%  week:-2% (2h0m)", []Gauge{{"session", 5, 0, false}, {"weekly", -2, 2 * h, true}}},
		{"week:7% (soon)  session:1% (3h44m)", []Gauge{{"weekly", 7, 0, false}, {"session", 1, 0, false}}},
		{"week:12% (6d1h)  week:50% (1m)  5h 0% left", []Gauge{{"weekly", 12, 6*24*h + h, true}, {"session", 0, 0, false}}},
		{"session:x%  week:%  mysession:5%  session: (03:44)  week:44m", nil},
	}
	for _, c := range cases {
		lines := []string{"● Done.", rule, "❯ ", rule, "  " + c.footer}
		if got := Gauges(lines); !slices.Equal(got, c.want) {
			t.Errorf("%q: got %+v, want %+v", c.footer, got, c.want)
		}
	}
	if got := Gauges([]string{"  ctx:16%  session:28% (03:44)"}); got != nil {
		t.Errorf("no prompt line: got %+v", got)
	}
	if got := Gauges([]string{"  session:28% (03:44)", "❯ "}); got != nil {
		t.Errorf("gauge above the prompt line: got %+v", got)
	}
}

func TestRead(t *testing.T) {
	loc := warsaw(t)
	at := func(mo time.Month, d, hr, mi int) int64 { return time.Date(2026, mo, d, hr, mi, 0, 0, loc).Unix() }

	limited := fixture(t, "2026-10-03/visible-wB-p1.txt")
	noWeek := replace(t, limited, "  week:0% (44m)", "")
	roomy := replace(t, limited, "week:0% (44m)", "week:12% (6d1h)")
	evening := fixture(t, "2026-10-03-evening/visible-wB-p1.txt")

	rule := strings.Repeat("─", 40)
	claude := func(notice, footer string) string {
		return strings.Join([]string{
			"● Working on it.",
			"  ⎿  " + notice,
			"     /upgrade to increase your usage limit.",
			"",
			rule, "❯ ", rule,
			"  ~/projects/a  main  Opus 5.5",
			"  " + footer,
		}, "\n") + "\n"
	}
	session := claude("You've hit your session limit · resets 3pm", "ctx:40%  cost:$2.10")
	datedRoomy := claude("You've hit your weekly limit · resets Oct 9, 3pm", "ctx:40%  week:50% (5d23h)")

	codex := func(notice string, resumed bool) string {
		lines := []string{notice, ""}
		if resumed {
			lines = append(lines, "› resume", "")
		}
		lines = append(lines, "» Ask Codex to do anything", "",
			"  GPT-6.1-Sol ultra · ~/projects/shop-backend · sb-42 · 5h 0% left · weekl…")
		return strings.Join(lines, "\n") + "\n"
	}
	usage := func(date string) string {
		return "■ You’ve hit your usage limit. Upgrade to Pro (https://openai.com/chatgpt/pricing), visit " +
			"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at " + date + "."
	}

	cases := []struct {
		name        string
		out         string
		now         int64
		state       State
		window      string
		reset       int64
		unconfirmed bool
	}{
		{"weekly limit before its reset", limited, at1522, Limited, "weekly", at1600, false},
		{"footer bound excludes tomorrow's 4pm", limited, at161640, Over, "weekly", at1600, false},
		{"no gauge, reset near", noWeek, at1522, Limited, "weekly", at1600, false},
		{"no gauge, reset a day out", noWeek, at161640, Limited, "weekly", atSun1600, true},
		{"gauge shows room", roomy, at161640, Over, "weekly", at1600, false},
		{"gauge shows room before the 4pm", roomy, at1522, Over, "weekly", at1600 - 86400, false},
		{"session limit inside its window", evening, at(10, 3, 17, 54), Limited, "session", at(10, 3, 19, 0), false},
		{"session notice six hours out", session, at(10, 3, 21, 0), Over, "session", at(10, 3, 15, 0), false},
		{"session notice ahead", session, at(10, 3, 12, 0), Limited, "session", at(10, 3, 15, 0), false},
		{"dated reset ahead but gauge shows room", datedRoomy, at1522, Over, "weekly", 0, false},
		{"codex date passed", codex(usage("Sep 27th, 2026 12:27 AM"), false), at1522, Over, "", at(9, 27, 0, 27), false},
		{"codex date ahead", codex(usage("Oct 9th, 2026 12:27 AM"), false), at1522, Limited, "", at(10, 9, 0, 27), false},
		{"codex no reset named", codex("■ Usage limit reached", false), at1522, Limited, "", 0, false},
		{"codex resumed", codex(usage("Oct 9th, 2026 12:27 AM"), true), at1522, None, "", 0, false},
		{"no notice", fixture(t, "2026-10-03/visible-wA-p1.txt"), at1522, None, "", 0, false},
	}
	for _, c := range cases {
		st := Read(c.out, c.now)
		if st.State != c.state || st.Window != c.window || st.Reset != c.reset || st.Unconfirmed != c.unconfirmed {
			t.Errorf("%s: got state %d window %q reset %s unconfirmed %v, want %d %q %s %v", c.name,
				st.State, st.Window, when.Format(st.Reset, when.Calendar), st.Unconfirmed,
				c.state, c.window, when.Format(c.reset, when.Calendar), c.unconfirmed)
		}
	}

	st := Read(limited, at1522)
	if st.Time != "4pm (Europe/Warsaw)" || st.Quote != "resets 4pm (Europe/Warsaw)" || st.Source != "" || len(st.Gauges) != 2 {
		t.Errorf("got %+v", st)
	}
}

// screens serves saved screens to Scan and counts the reads.
type screens struct {
	mu    sync.Mutex
	out   map[string]string
	reads map[string]int
}

func (s *screens) read(pane string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads[pane]++
	return s.out[pane]
}

func TestScan(t *testing.T) {
	const h, m = time.Hour, time.Minute
	warsaw(t)
	agents := []Agent{{"wA:p1", "claude"}, {"wB:p1", "claude"}, {"wB:p3", "codex"}, {"wB:p2", "claude"}}
	wA := fixture(t, "2026-10-03/visible-wA-p1.txt")
	wB := fixture(t, "2026-10-03/visible-wB-p1.txt")
	codex := fixture(t, "2026-10-03/visible-wB-p3.txt")

	cases := []struct {
		name   string
		wA     string
		state  State
		source string
		reset  int64
		gauges []Gauge
	}{
		{"own weekly gauge vetoes", wA, None, "", 0, []Gauge{{"weekly", 11, h + 58*m, true}}},
		{"no gauge to veto", replace(t, wA, "  week:11% (1h58m)", ""), Account, "wB:p1", at1600, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &screens{
				out:   map[string]string{"wA:p1": c.wA, "wB:p1": wB, "wB:p3": codex, "wB:p2": wB},
				reads: map[string]int{},
			}
			got := Scan(agents, "wB:p2", at1522, s.read)

			if b := got["wB:p1"]; b.State != Limited || b.Source != "wB:p1" || b.Reset != at1600 || b.Window != "weekly" {
				t.Errorf("wB:p1: got %+v", b)
			}
			a := got["wA:p1"]
			if a.State != c.state || a.Source != c.source || a.Reset != c.reset || !slices.Equal(a.Gauges, c.gauges) {
				t.Errorf("wA:p1: got %+v", a)
			}
			if c.state == Account && (a.Window != "weekly" || a.Time != "4pm (Europe/Warsaw)") {
				t.Errorf("wA:p1: got %+v, want wB:p1's notice", a)
			}
			if p3 := got["wB:p3"]; p3.State != None {
				t.Errorf("wB:p3: got %+v", p3)
			}
			if _, ok := got["wB:p2"]; ok || len(got) != 3 {
				t.Errorf("got %d statuses, want 3 and none for self", len(got))
			}
			want := map[string]int{"wA:p1": 1, "wB:p1": 1, "wB:p3": 1}
			if len(s.reads) != len(want) || s.reads["wA:p1"] != 1 || s.reads["wB:p1"] != 1 || s.reads["wB:p3"] != 1 {
				t.Errorf("reads = %v, want %v", s.reads, want)
			}
		})
	}
}

func TestScanAccountTakesTheSoonestReset(t *testing.T) {
	warsaw(t)
	rule := strings.Repeat("─", 40)
	draw := func(notice string) string {
		return strings.Join([]string{"  ⎿  " + notice, "", rule, "❯ ", rule, "  ctx:40%"}, "\n") + "\n"
	}
	s := &screens{
		out: map[string]string{
			"wA:p1": draw("You've hit your weekly limit · resets 5pm (Europe/Warsaw)"),
			"wA:p2": draw("You've hit your weekly limit · resets 4pm (Europe/Warsaw)"),
			"wA:p3": "● Done.\n" + rule + "\n❯ \n" + rule + "\n",
		},
		reads: map[string]int{},
	}
	got := Scan([]Agent{{"wA:p1", "claude"}, {"wA:p2", "claude"}, {"wA:p3", "claude"}}, "", at1522, s.read)
	if a := got["wA:p3"]; a.State != Account || a.Source != "wA:p2" || a.Reset != at1600 {
		t.Errorf("got %+v", a)
	}
}

func TestBefore(t *testing.T) {
	order := []Status{
		{State: Over},
		{State: Limited, Reset: at1600},
		{State: Limited, Reset: at1600 + 3600},
		{State: Limited},
	}
	for i := range order {
		for j := range order {
			if got := Before(order[i], order[j]); got != (i < j) {
				t.Errorf("Before(%+v, %+v) = %v", order[i], order[j], got)
			}
		}
	}
}

func TestScanTriesOtherHoldersAfterGaugeVeto(t *testing.T) {
	warsaw(t)
	draw := func(n, footer string) string { return n + "\n\n❯ \n────────\n" + footer }
	s := &screens{out: map[string]string{
		"session": draw("You've hit your session limit · resets 4pm", "session:0% (00:38)"),
		"weekly":  draw("You've hit your weekly limit · resets 5pm", "week:0% (1h38m)"),
		"target":  draw("● Done.", "session:80% (00:38) week:0% (1h38m)"),
	}, reads: map[string]int{}}
	got := Scan([]Agent{{"session", "claude"}, {"weekly", "claude"}, {"target", "claude"}}, "", at1522, s.read)["target"]
	if got.State != Account || got.Source != "weekly" || got.Reset != at1600+3600 {
		t.Fatalf("got %+v, want weekly account limit from the non-vetoed holder", got)
	}
}

func TestReadYearBoundaryNotices(t *testing.T) {
	loc := warsaw(t)
	at := func(y int, mo time.Month, d, hr int) int64 { return time.Date(y, mo, d, hr, 0, 0, 0, loc).Unix() }
	cases := []struct {
		name, reset string
		now         int64
		state       State
		want        int64
	}{
		{"upcoming new year", "Jan 1, 3pm", at(2026, 12, 31, 20), Limited, at(2027, 1, 1, 15)},
		{"stale previous year", "Dec 31, 3pm", at(2027, 1, 1, 12), Over, at(2026, 12, 31, 15)},
		{"older stale previous year", "Dec 31, 3pm", at(2027, 1, 15, 12), Over, at(2026, 12, 31, 15)},
		{"stale same year", "Sep 28, 3pm", at(2026, 10, 3, 12), Over, at(2026, 9, 28, 15)},
		{"explicit past year", "Jan 1, 2026 3pm", at(2026, 12, 31, 20), Over, at(2026, 1, 1, 15)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := "You've hit your weekly limit · resets " + c.reset + "\n\n❯ \n────────\nweek:0% (19h0m)"
			st := Read(out, c.now)
			if st.State != c.state || st.Reset != c.want {
				t.Fatalf("got %+v, want state=%d reset=%s", st, c.state, when.Format(c.want, when.Calendar))
			}
		})
	}
}

func TestReadPreviousResetAcrossFallBack(t *testing.T) {
	loc := warsaw(t)
	now := time.Date(2026, 10, 25, 23, 30, 0, 0, loc).Unix()
	want := time.Date(2026, 10, 25, 0, 15, 0, 0, loc).Unix()
	st := Read("You've hit your session limit · resets 12:15am\n\n❯ \n────────\nsession:80% (01:00)", now)
	if st.State != Over || st.Reset != want {
		t.Fatalf("got %+v, want Over with %s", st, when.Format(want, when.Calendar))
	}
}
