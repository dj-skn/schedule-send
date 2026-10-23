package when

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func warsaw(t *testing.T) *time.Location {
	t.Helper()
	t.Setenv("TZ", "Europe/Warsaw")
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	prev := Loc
	Loc = loc
	t.Cleanup(func() { Loc = prev })
	return loc
}

func TestResolve(t *testing.T) {
	loc := warsaw(t)
	at := func(y int, mo time.Month, d, h, mi, s int) int64 {
		return time.Date(y, mo, d, h, mi, s, 0, loc).Unix()
	}
	now := at(2026, 10, 3, 15, 22, 0)

	cases := []struct {
		in   string
		want int64
	}{
		{"15:50", at(2026, 10, 3, 15, 50, 0)},
		{"03:30:15", at(2026, 10, 4, 3, 30, 15)},
		{"12:27 AM", at(2026, 10, 4, 0, 27, 0)},
		{"3pm", at(2026, 10, 4, 15, 0, 0)},
		{"4pm", at(2026, 10, 3, 16, 0, 0)},
		{"4pm (Europe/Warsaw)", at(2026, 10, 3, 16, 0, 0)},
		{"4pm (UTC)", at(2026, 10, 3, 18, 0, 0)},
		{"Sep 27th, 2026 12:27 AM", at(2026, 9, 27, 0, 27, 0)},
		{"Sep 28, 3pm", at(2026, 9, 28, 15, 0, 0)},
		{"2026-06-24 15:50:00", at(2026, 6, 24, 15, 50, 0)},
		{"2026-06-24T15:50:00", at(2026, 6, 24, 15, 50, 0)},
	}
	for _, c := range cases {
		got, ok := Resolve(c.in, now)
		if !ok || got != c.want {
			t.Errorf("Resolve(%q) = %s ok=%v, want %s", c.in,
				Format(got, Calendar), ok, Format(c.want, Calendar))
		}
	}
}

func TestResolveRejects(t *testing.T) {
	warsaw(t)
	for _, in := range []string{"garbage", "4pm (Mars/Base)", "25:99"} {
		if got, ok := Resolve(in, 1791033720); ok {
			t.Errorf("Resolve(%q) = %d, want unparseable", in, got)
		}
	}
}

func TestTimeOnly(t *testing.T) {
	cases := map[string]bool{
		"4pm (Europe/Warsaw)":     true,
		"3:30pm":                  true,
		"15:50":                   true,
		"Sep 28, 3pm":             false,
		"Sep 27th, 2026 12:27 AM": false,
		"2026-10-04T09:00:00":     false,
	}
	for in, want := range cases {
		if got := TimeOnly(in); got != want {
			t.Errorf("TimeOnly(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveAcrossDST(t *testing.T) {
	// Summer time ends on 2026-10-25. Advancing 24 hours from noon on the 24th
	// would land on 11:00; re-parsing against the 25th keeps it at noon.
	loc := warsaw(t)
	now := time.Date(2026, 10, 24, 13, 0, 0, 0, loc).Unix()
	want := time.Date(2026, 10, 25, 12, 0, 0, 0, loc).Unix()
	if got, ok := Resolve("12:00", now); !ok || got != want {
		t.Errorf("got %s, want %s", Format(got, Calendar), Format(want, Calendar))
	}
}

func TestResolveRelative(t *testing.T) {
	// date(1) reads "+30 min" against the wall clock, not the given now.
	warsaw(t)
	got, ok := Resolve("+30 min", 0)
	want := time.Now().Add(30 * time.Minute).Unix()
	if !ok || got < want-5 || got > want+5 {
		t.Errorf("got %d, want about %d", got, want)
	}
}

func TestDelay(t *testing.T) {
	cases := map[int64]string{
		0: "0m", 1: "1m", 60: "1m", 61: "2m", 2654: "45m",
		3599: "1h 0m", 3661: "1h 2m", 86400: "1d 0h", 90061: "1d 1h",
	}
	for in, want := range cases {
		if got := Delay(in); got != want {
			t.Errorf("Delay(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	warsaw(t)
	e := int64(1791036000)
	for layout, want := range map[string]string{
		DayMinute: "2026-10-03 16:00", WeekdayMinute: "Sat 16:00",
		WeekdayFull: "Sat 2026-10-03 16:00:00", Calendar: "2026-10-03 16:00:00",
		Stamp: "20261003-160000",
	} {
		if got := Format(e, layout); got != want {
			t.Errorf("Format(%q) = %q, want %q", layout, got, want)
		}
	}
}

func TestResolveResetYearAndHorizon(t *testing.T) {
	loc := warsaw(t)
	now := time.Date(2026, 12, 31, 20, 0, 0, 0, loc).Unix()
	cases := []struct {
		in      string
		horizon int64
		want    time.Time
	}{
		{"Jan 1, 3pm", 7 * 86400, time.Date(2027, 1, 1, 15, 0, 0, 0, loc)},
		{"Jan 1, 3pm", 3600, time.Date(2026, 1, 1, 15, 0, 0, 0, loc)},
		{"Jan 1, 2026 3pm", 7 * 86400, time.Date(2026, 1, 1, 15, 0, 0, 0, loc)},
		{"Jan 1, 3pm (UTC)", 7 * 86400, time.Date(2027, 1, 1, 15, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got, ok := ResolveReset(c.in, now, c.horizon)
		if !ok || got != c.want.Unix() {
			t.Errorf("ResolveReset(%q,%d)=%d ok=%v want=%d", c.in, c.horizon, got, ok, c.want.Unix())
		}
	}
	// The omitted year comes from the named zone, even if our local year differs.
	now = time.Date(2027, 1, 1, 0, 30, 0, 0, loc).Unix()
	want := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC).Unix()
	if got, ok := ResolveReset("Dec 31, 11pm (UTC)", now, 7*86400); !ok || got != want {
		t.Errorf("zone's year: got=%d ok=%v want=%d", got, ok, want)
	}
}

func TestPreviousUsesCalendarDays(t *testing.T) {
	loc := warsaw(t)
	cases := []struct {
		in        string
		now, want time.Time
	}{
		{"12:15am", time.Date(2026, 10, 25, 23, 30, 0, 0, loc), time.Date(2026, 10, 25, 0, 15, 0, 0, loc)},
		{"11pm", time.Date(2026, 3, 29, 22, 0, 0, 0, loc), time.Date(2026, 3, 28, 23, 0, 0, 0, loc)},
		{"11pm (UTC)", time.Date(2026, 10, 25, 0, 30, 0, 0, loc), time.Date(2026, 10, 23, 23, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got, ok := Previous(c.in, c.now.Unix()); !ok || got != c.want.Unix() {
			t.Errorf("Previous(%q)=%d ok=%v want=%d", c.in, got, ok, c.want.Unix())
		}
	}
	if _, ok := Previous("12pm (Mars/Base)", 0); ok {
		t.Fatal("accepted unknown zone")
	}
}

func TestDateSubprocessTimeout(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "date")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	previous := dateTool
	dateTool = tool
	t.Cleanup(func() { dateTool = previous })
	start := time.Now()
	if _, ok := Resolve("garbage", 0); ok {
		t.Fatal("hung date returned a parsed time")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("date took %s; timeout should bound it", elapsed)
	}
}

func TestResolveShortOffsets(t *testing.T) {
	warsaw(t)
	for in, d := range map[string]time.Duration{
		"+30": 30 * time.Minute, "+30m": 30 * time.Minute, "+ 30": 30 * time.Minute,
		"+45 m": 45 * time.Minute, "+2h": 2 * time.Hour, " +1h ": time.Hour,
	} {
		got, ok := Resolve(in, 0)
		want := time.Now().Add(d).Unix()
		if !ok || got < want-5 || got > want+5 {
			t.Errorf("Resolve(%q) = %d, %v; want about %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"+30x", "30m", "+-5"} {
		if _, ok := Resolve(in, 0); ok {
			t.Errorf("Resolve(%q) was read", in)
		}
	}
}
