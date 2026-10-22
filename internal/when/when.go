// Package when turns a send time as typed or as printed in a usage-limit notice
// into an epoch, and formats epochs and delays for display.
//
// Parsing is left to GNU date(1): it is what accepts "+30 min" and
// "Sep 27 1am" at the confirmation prompt. macOS has it as gdate.
// This package decides what date(1) is handed.
package when

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Loc is the zone epochs are formatted in. It is the process's local zone,
// the same one date(1) parses in; tests set both.
var Loc = time.Local

// Layouts for Format, named after the date(1) formats they replace.
const (
	DayMinute      = "2006-01-02 15:04"        // %F %H:%M
	WeekdayMinute  = "Mon 15:04"               // %a %H:%M
	WeekdayFull    = "Mon 2006-01-02 15:04:05" // %a %F %H:%M:%S
	Calendar       = "2006-01-02 15:04:05"     // %F %T
	Stamp          = "20060102-150405"         // %Y%m%d-%H%M%S
	Day            = "2006-01-02"              // %F
	Minute         = "15:04"                   // %H:%M
	MonthDayMinute = "Jan 2 15:04"             // %b %-d %H:%M
)

// Format formats an epoch in Loc.
func Format(epoch int64, layout string) string {
	return time.Unix(epoch, 0).In(Loc).Format(layout)
}

// Delay formats a delay in seconds as "2h 5m", rounded up to the minute.
func Delay(secs int64) string {
	m := (secs + 59) / 60
	switch {
	case m >= 1440:
		return strconv.FormatInt(m/1440, 10) + "d " + strconv.FormatInt(m%1440/60, 10) + "h"
	case m >= 60:
		return strconv.FormatInt(m/60, 10) + "h " + strconv.FormatInt(m%60, 10) + "m"
	default:
		return strconv.FormatInt(m, 10) + "m"
	}
}

var (
	isoT     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T`)
	zoned    = regexp.MustCompilePOSIX(`^(.*[^ ]) *\(([A-Za-z_]+(/[A-Za-z0-9_+-]+)*)\)$`)
	ordinal  = regexp.MustCompile(`([0-9])(st|nd|rd|th)`)
	clock24  = regexp.MustCompile(`^[0-9]{1,2}:[0-9]{2}(:[0-9]{2})?$`)
	clock12  = regexp.MustCompile(`^[0-9]{1,2}(:[0-9]{2}(:[0-9]{2})?)? ?[AaPp][Mm]$`)
	yearless = regexp.MustCompile(`^([A-Z][a-z]{2} [0-9]{1,2}) ([0-9]{1,2}(:[0-9]{2})? ?[AaPp][Mm])$`)
	short    = regexp.MustCompile(`^\+ *([0-9]+) *([mh]?)$`)
	dateTool = DateTool
)

// Resolve turns a send time into an epoch. ok is false when date(1) cannot
// read it.
//
// A value with no date part means its next occurrence. That includes 12-hour
// forms such as "12:27 AM" and "3pm": handed to date(1) whole they resolve to
// today even once today's has passed. When it has, the time is re-parsed
// against tomorrow's calendar date rather than advanced by 86400, so a DST
// transition between the two days cannot shift the result by an hour.
func Resolve(at string, now int64) (epoch int64, ok bool) {
	at, zone := normalise(at)
	prefix := ""
	if zone != "" {
		prefix = `TZ="` + zone + `" `
		// glibc reads a zone it does not know as UTC, without a word.
		if !zoneKnown(zone) {
			return 0, false
		}
	}

	if !isClock(at) {
		return parse(prefix + at)
	}
	// "Today" is the zone's own date, which differs from ours near midnight.
	day := Format(now, Day)
	if zone != "" {
		if day, ok = dayIn(zone, now); !ok {
			return 0, false
		}
	}
	epoch, ok = parse(prefix + day + " " + at)
	if ok && epoch <= now {
		epoch, ok = parse(prefix + nextDay(day) + " " + at)
	}
	return epoch, ok
}

// ResolveReset resolves a usage-limit notice. Month/day notices omit the year:
// use the notice's current year, and consider an adjacent year only around New
// Year when a future interpretation fits the limit's reset horizon. A month/day
// more than a week ahead must instead be a stale notice from the previous year:
// usage windows cannot extend beyond a week. Explicit-year notices and other
// accepted date expressions keep Resolve's semantics.
func ResolveReset(at string, now, horizon int64) (int64, bool) {
	text, zone := normalise(at)
	m := yearless.FindStringSubmatch(text)
	if m == nil {
		return Resolve(at, now)
	}
	day, prefix, ok := dateContext(zone, now)
	if !ok {
		return 0, false
	}
	year, err := strconv.Atoi(day[:4])
	if err != nil {
		return 0, false
	}
	resolve := func(y int) (int64, bool) {
		return parse(prefix + m[1] + " " + strconv.Itoa(y) + " " + m[2])
	}
	epoch, ok := resolve(year)
	if !ok {
		return 0, false
	}
	if epoch <= now {
		if next, valid := resolve(year + 1); valid && next > now && next-now <= horizon {
			return next, true
		}
	} else if epoch-now > 7*24*3600 {
		if prior, valid := resolve(year - 1); valid && prior <= now {
			return prior, true
		}
	}
	return epoch, true
}

// Previous returns the most recent occurrence of a clock time, at or before
// now. Calendar days, rather than 24-hour subtraction, keep this correct on
// days with a daylight-saving transition and in a notice's explicit zone.
func Previous(at string, now int64) (int64, bool) {
	at, zone := normalise(at)
	if !isClock(at) {
		return 0, false
	}
	day, prefix, ok := dateContext(zone, now)
	if !ok {
		return 0, false
	}
	epoch, ok := parse(prefix + day + " " + at)
	if ok && epoch > now {
		epoch, ok = parse(prefix + shiftDay(day, -1) + " " + at)
	}
	return epoch, ok
}

// dateContext is the date and GNU date zone prefix for an instant.
func dateContext(zone string, now int64) (day, prefix string, ok bool) {
	day = Format(now, Day)
	if zone == "" {
		return day, "", true
	}
	if !zoneKnown(zone) {
		return "", "", false
	}
	day, ok = dayIn(zone, now)
	return day, `TZ="` + zone + `" `, ok
}

// TimeOnly reports whether at is a clock time with no date, which Resolve
// takes as its next occurrence.
func TimeOnly(at string) bool {
	at, _ = normalise(at)
	return isClock(at)
}

func isClock(at string) bool { return clock24.MatchString(at) || clock12.MatchString(at) }

// normalise rewrites at into a form date(1) accepts, with a bracketed zone
// split off.
func normalise(at string) (string, string) {
	// "+30" and "+30m" are minutes and "+2h" hours: the short ways of writing
	// an offset, which date(1) does not read.
	if m := short.FindStringSubmatch(strings.TrimSpace(at)); m != nil {
		unit := " min"
		if m[2] == "h" {
			unit = " hour"
		}
		return "+" + m[1] + unit, ""
	}
	// Only an ISO timestamp has its T separator swapped out; anywhere else a
	// capital T is part of the text, as in "Tue 3pm".
	if isoT.MatchString(at) {
		at = strings.Replace(at, "T", " ", 1)
	}

	// Agents print reset times in forms date(1) rejects as written: an ordinal
	// day ("Sep 27th"), a comma before the time ("Sep 28, 3pm"), and a zone in
	// brackets ("3pm (Europe/Warsaw)"). They are normalised here rather than in
	// the detector, so a time copied out of a notice parses the same as one
	// read from it.
	zone := ""
	if m := zoned.FindStringSubmatch(at); m != nil {
		at, zone = m[1], m[2]
	}
	return ordinal.ReplaceAllString(strings.ReplaceAll(at, ",", ""), "${1}"), zone
}

func zoneKnown(zone string) bool {
	dir := os.Getenv("TZDIR")
	if dir == "" {
		dir = "/usr/share/zoneinfo"
	}
	fi, err := os.Stat(filepath.Join(dir, zone))
	return err == nil && fi.Mode().IsRegular()
}

func parse(s string) (int64, bool) {
	out, err := dateOutput([]string{"-d", s, "+%s"}, nil)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n, err == nil
}

func dayIn(zone string, now int64) (string, bool) {
	out, err := dateOutput([]string{"-d", "@" + strconv.FormatInt(now, 10), "+%F"}, append(os.Environ(), "TZ="+zone))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func nextDay(day string) string { return shiftDay(day, 1) }

func shiftDay(day string, days int) string {
	t, err := time.Parse(Day, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, days).Format(Day)
}

// Bound external date execution, including descendants that retain its pipes.
func dateOutput(args []string, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, dateTool, args...)
	cmd.Env = env
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Output()
}
