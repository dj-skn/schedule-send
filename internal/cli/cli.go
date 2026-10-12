// Package cli is schedule-send's command line: argument parsing, target
// selection, the send time, and arming the job.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"schedule-send/internal/herdr"
	"schedule-send/internal/installpath"
	"schedule-send/internal/job"
	"schedule-send/internal/limit"
	"schedule-send/internal/show"
	"schedule-send/internal/target"
	"schedule-send/internal/term"
	"schedule-send/internal/when"
	"schedule-send/internal/worker"
)

// failure ends the command: msg goes to stderr, code is the exit status.
type failure struct {
	msg  string
	code int
	kind string
}

func (f failure) Error() string { return f.msg }

func die(format string, a ...any) error { return fail("failed", format, a...) }

func fail(kind, format string, a ...any) error {
	return failure{msg: fmt.Sprintf(format, a...), code: 1, kind: kind}
}

// What a run without --at sends when no payload was given: resuming after the
// limit is what a detected time is for.
const resumeMessage = "resume"

// A notice names its reset to the minute, and the reset lands somewhere inside
// that minute — a notice reading 12:27 AM was for a reset at 00:27:52 — so the
// first moment certainly past it is one minute on.
const limitMargin = 60

type options struct {
	pane, cwd, kind, at          string
	message, messageFile, keys   string
	pick, ifBusy, recover, grace string
	panesOnly, allowSelf, force  bool
	quiet, dryRun, persistent    bool
	auto, haveMessage, defaulted bool
	json                         bool
	operation, operationID       string
}

type app struct {
	o              options
	stdout, stderr io.Writer
	stdinTTY       func() bool
	stdoutTTY      func() bool
	stderrTTY      func() bool
	openTTY        func() (io.ReadWriteCloser, error)
	clock          func() int64 // the time now, as an epoch; run sets it when nil
	paths          job.Paths
	sys            job.Systemctl
	h              herdr.Client
	selfPane       string                 // HERDR_PANE_ID
	self           string                 // preview executable, or the validated installed path
	installedPath  func() (string, error) // private injection for isolated tests
	statusTimeouts statusTimeouts         // private deadlines for isolated tests

	rows []target.Row
	tgt  target.Row
	now  int64 // when the run started; screens are judged at this moment

	lim map[string]limit.Status // set by limits
}

// Main runs the command and returns its exit status.
func Main(args []string) int {
	a := &app{
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		stdinTTY:  func() bool { return term.IsTerminal(os.Stdin.Fd()) },
		stdoutTTY: func() bool { return term.IsTerminal(os.Stdout.Fd()) },
		stderrTTY: func() bool { return term.IsTerminal(os.Stderr.Fd()) },
		openTTY: func() (io.ReadWriteCloser, error) {
			return os.OpenFile("/dev/tty", os.O_RDWR, 0)
		},
		paths:    job.DefaultPaths(),
		selfPane: os.Getenv("HERDR_PANE_ID"),
	}
	return a.main(args)
}

func (a *app) main(args []string) int {
	if len(args) > 0 && args[0] == "__fire" {
		if len(args) < 2 || args[1] == "" {
			fmt.Fprintln(a.stderr, "__fire needs an id")
			return 1
		}
		return worker.Fire(args[1])
	}
	// Noticed before the arguments are walked, so --list --json still counts.
	for _, arg := range args {
		if arg == "--json" {
			a.o.json = true
			break
		}
	}
	err := a.run(args)
	var f failure
	switch {
	case err == nil:
		return 0
	case errors.As(err, &f):
		if f.code == 0 || f.msg == "" {
			return f.code
		}
		if a.o.json {
			a.writeError(f)
		} else {
			fmt.Fprintln(a.stderr, f.msg)
		}
		return f.code
	default:
		if a.o.json {
			a.writeError(failure{msg: err.Error(), code: 1})
		} else {
			fmt.Fprintln(a.stderr, err)
		}
		return 1
	}
}

// writeError prints a failure as {"error":{"code","message"}}, the shape herdr
// uses. The code names the kind of failure; the exit status is unchanged.
func (a *app) writeError(f failure) {
	code := f.kind
	if code == "" {
		code = "failed"
	}
	emit(a.stderr, map[string]any{"error": map[string]string{"code": code, "message": f.msg}})
}

// emit writes v as one JSON line, without escaping < and >.
func emit(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// note and warn are the asides on stderr. Under --json each is its own object,
// so stderr stays something a script can read.
func (a *app) note(msg string) {
	if a.o.json {
		emit(a.stderr, struct {
			Note string `json:"note"`
		}{msg})
		return
	}
	fmt.Fprintln(a.stderr, msg)
}

func (a *app) warn(msg string) {
	if a.o.json {
		emit(a.stderr, struct {
			Warning string `json:"warning"`
		}{msg})
		return
	}
	fmt.Fprintln(a.stderr, msg)
}

func (a *app) parse(args []string) error {
	o := &a.o
	o.pick, o.ifBusy, o.recover, o.grace = "unique", "wait", "keys", "4"
	scheduling := false
	operation := func(name string) error {
		if o.operation != "" {
			return fail("usage", "Use only one operation: %s and %s cannot be combined.", o.operation, name)
		}
		o.operation = name
		return nil
	}
	for i := 0; i < len(args); {
		flag := args[i]
		switch flag {
		case "--json", "--targets", "--list", "--cancel", "--status", "--help", "-h":
		default:
			scheduling = true
		}
		value := func() (string, error) {
			v := ""
			if i+1 < len(args) {
				v = args[i+1]
			}
			if v == "" {
				return "", fail("usage", "Missing value for %s.", flag)
			}
			if strings.HasPrefix(v, "--") {
				return "", fail("usage", "Missing value for %s (got the flag '%s').", flag, v)
			}
			i += 2
			return v, nil
		}
		var err error
		set := func(dst *string) { *dst, err = value() }
		on := func(dst *bool) { *dst = true; i++ }

		switch flag {
		case "--pane":
			set(&o.pane)
		case "--cwd":
			set(&o.cwd)
		case "--kind":
			set(&o.kind)
		case "--at":
			set(&o.at)
		case "--auto":
			on(&o.auto)
		case "--message":
			set(&o.message)
			o.haveMessage = true
		case "--message-file":
			set(&o.messageFile)
			o.haveMessage = true
		case "--keys":
			set(&o.keys)
		case "--pick":
			set(&o.pick)
		case "--if-busy":
			set(&o.ifBusy)
		case "--recover":
			set(&o.recover)
		case "--submit-grace":
			set(&o.grace)
		case "--enter-only":
			o.keys = "enter"
			i++
		case "--panes-only":
			on(&o.panesOnly)
		case "--allow-self":
			on(&o.allowSelf)
		case "--force":
			on(&o.force)
		case "--quiet":
			on(&o.quiet)
		case "--persistent":
			on(&o.persistent)
		case "--dry-run":
			on(&o.dryRun)
		case "--json":
			on(&o.json)
		case "--targets", "--list":
			err = operation(flag)
			i++
		case "--cancel", "--status":
			if err = operation(flag); err == nil {
				o.operationID, err = value()
			}
		case "-h", "--help":
			err = operation("--help")
			i++
		default:
			if a.o.json {
				return failure{code: 2, kind: "usage", msg: "Unknown argument: " + flag}
			}
			fmt.Fprintln(a.stderr, "Unknown argument: "+flag)
			fmt.Fprint(a.stderr, usage)
			return failure{code: 2, kind: "usage"}
		}
		if err != nil {
			return err
		}
	}
	if o.operation != "" && scheduling {
		return fail("usage", "Scheduling options cannot be combined with %s.", o.operation)
	}
	return nil
}

var digits = regexp.MustCompile(`^[0-9]+$`)

func (a *app) run(args []string) (retErr error) {
	if err := a.parse(args); err != nil {
		return err
	}
	switch a.o.operation {
	case "--targets":
		return a.showTargets()
	case "--list":
		a.list()
		return nil
	case "--cancel":
		return a.cancel(a.o.operationID)
	case "--status":
		return a.status(a.o.operationID)
	case "--help":
		fmt.Fprint(a.stdout, usage)
		return nil
	}
	o := &a.o

	if o.pick != "unique" && o.pick != "best" {
		return die("--pick must be 'unique' or 'best'.")
	}
	if o.ifBusy != "wait" && o.ifBusy != "abort" {
		return die("--if-busy must be 'wait' or 'abort'.")
	}
	if o.recover != "none" && o.recover != "keys" && o.recover != "focus" {
		return die("--recover must be 'none', 'keys', or 'focus'.")
	}
	grace, err := strconv.Atoi(o.grace)
	if !digits.MatchString(o.grace) || err != nil || grace < 1 || grace > 60 {
		return die("--submit-grace must be a whole number of seconds from 1 to 60.")
	}
	if o.at != "" && o.auto {
		return die("Use either --at or --auto, not both.")
	}

	// Without --at the send time comes from the target's usage-limit notice,
	// and a detected time is only taken unseen when --auto says so.
	detect := o.at == ""
	if detect && !o.auto && !a.stdinTTY() {
		return die("No --at, and no terminal to confirm a detected time on. Pass --at, or --auto to take the detected time unconfirmed.")
	}

	if o.message != "" && o.messageFile != "" {
		return die("Use either --message or --message-file, not both.")
	}
	if o.haveMessage && o.keys != "" {
		return die("Use either a message or --keys, not both.")
	}
	if o.messageFile != "" {
		fi, err := os.Stat(o.messageFile)
		if err != nil || !fi.Mode().IsRegular() {
			return die("Message file does not exist: %s", o.messageFile)
		}
		b, err := os.ReadFile(o.messageFile)
		if err != nil {
			return die("Message file does not exist: %s", o.messageFile)
		}
		o.message = strings.TrimRight(string(b), "\n")
	}
	if detect && !o.haveMessage && o.keys == "" {
		o.message = resumeMessage
		o.defaulted = true
	}
	if o.keys == "" && o.message == "" {
		return die("Nothing to send. Pass --message, --message-file, or --keys.")
	}

	if !o.dryRun {
		resolve := a.installedPath
		if resolve == nil {
			resolve = installpath.Resolve
		}
		installed, err := resolve()
		if err != nil {
			return die("Cannot arm a job: %v. Run tools/install from the main checkout, then invoke the installed schedule-send.", err)
		}
		a.self = installed
		fi, err := os.Stat("/usr/bin/env")
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
			return die("/usr/bin/env is required to execute scheduled jobs (GNU coreutils).")
		}
	}

	if _, err := exec.LookPath("systemctl"); err != nil {
		return die("systemctl is required.")
	}
	// is-system-running exits non-zero for "degraded", which merely means some
	// unrelated unit failed earlier. Only a manager we cannot talk to at all is
	// a real blocker, and that prints nothing.
	if a.sys.Output("is-system-running") == "" {
		return die("systemd --user is not available for this account.")
	}
	if err := a.locateHerdr(); err != nil {
		return err
	}
	if o.cwd != "" {
		o.cwd = target.NormPath(o.cwd)
	}

	a.rows, err = a.discoverTargets()
	if err != nil {
		return err
	}
	if len(a.rows) == 0 {
		return die("Herdr reported no panes.")
	}

	if err := a.startClock(); err != nil {
		return err
	}
	a.now = a.clock()
	now := a.now

	if o.pane != "" {
		found := false
		for _, r := range a.rows {
			if r.Pane == o.pane {
				a.tgt, found = r, true
				break
			}
		}
		if !found {
			return die("No such pane: %s", o.pane)
		}
	} else if a.tgt, err = a.chooseTarget(); err != nil {
		return err
	}
	t := a.tgt

	if t.Pane == a.selfPane && !o.allowSelf {
		return die("Refusing to target the pane this command runs in. Pass --allow-self to override.")
	}
	if o.defaulted && !t.IsAgent() {
		return die("Pane %s is a plain shell, where the default '%s' would run as a command. Pass --message.", t.Pane, resumeMessage)
	}
	if t.IsAgent() && t.Status == "blocked" && o.keys == "" {
		a.warn(fmt.Sprintf("Warning: agent in %s is blocked on a prompt right now.", t.Pane))
	}

	var runAt int64
	if detect {
		if runAt, err = a.detectAt(now); err != nil {
			return err
		}
		// A confirmation can sit unanswered for a while, so "already passed"
		// and the delay are judged from the moment it was answered.
		now = a.clock()
	} else {
		var ok bool
		if runAt, ok = when.Resolve(o.at, now); !ok {
			return die("Could not parse --at '%s'. Use HH:MM[:SS], H[:MM]am/pm, or YYYY-MM-DD HH:MM:SS.", o.at)
		}
	}
	if runAt <= now {
		return die("Target time has already passed: %s", when.Format(runAt, when.Calendar))
	}
	if !detect && t.IsAgent() && o.keys == "" {
		a.warnBeforeReset(runAt)
	}

	// systemd is handed the resolved wall-clock time, so what it fires on is
	// exactly what we printed and what date(1) computed.
	calendar := when.Format(runAt, when.Calendar)
	delay := runAt - now
	id := when.Format(runAt, when.Stamp) + "-" + alnum(t.Pane)
	unit := job.UnitPrefix + "-" + id

	// The id is the fire time plus the pane, so a second send to the same pane
	// at the same second would overwrite the first job's state and units while
	// its timer is still armed.
	if _, err := os.Stat(a.paths.StatePath(id)); err == nil {
		return die("A send to %s at %s is already scheduled (%s). Cancel it with --cancel %s, or pick another time.", t.Pane, calendar, id, id)
	}

	mode, payload := "text", firstLine(o.message)
	if o.keys != "" {
		mode, payload = "keys", "keys: "+o.keys
	}
	sched := show.Scheduled{
		DryRun: o.dryRun, ID: id, Unit: unit,
		Kind: t.Kind, Pane: t.Pane, Agent: t.Agent, Status: t.Status, Cwd: t.Cwd,
		At: calendar, RunAt: runAt, Delay: delay,
		Mode: mode, Payload: payload, Message: o.message, Keys: o.keys,
		Persistent: o.persistent, IfBusy: o.ifBusy, Recover: o.recover, Grace: o.grace,
		UnitFiles: a.paths.UnitDir + "/" + unit + ".{service,timer}",
		WouldRun:  a.selfPath() + " __fire " + id,
	}

	if o.dryRun {
		a.receipt(sched, now)
		return nil
	}

	serviceText, err := job.ServiceUnit(t.Pane, calendar, a.selfPath(), id)
	if err != nil {
		return die("Could not render the service: %v", err)
	}

	lock, err := a.paths.Lock(id)
	if err != nil {
		return die("Could not reserve %s: %v", id, err)
	}
	defer lock.Close()
	// Recheck while holding the reservation; the preview check is not a lock.
	if _, err := os.Stat(a.paths.StatePath(id)); err == nil {
		return die("A send to %s at %s is already scheduled (%s). Cancel it with --cancel %s, or pick another time.", t.Pane, calendar, id, id)
	} else if !errors.Is(err, os.ErrNotExist) {
		return die("Could not inspect %s: %v", id, err)
	}
	a.paths.Sweep(a.sys)
	state := job.State{
		ID: id, Kind: t.Kind, PaneID: t.Pane, Agent: t.Agent, Cwd: t.Cwd, At: calendar,
		Mode: mode, Keys: o.keys, IfBusy: o.ifBusy, Force: b2i(o.force), Quiet: b2i(o.quiet),
		Recover: o.recover, Grace: grace, HerdrBin: a.h.Bin, Unit: unit, RunAt: runAt,
	}
	var msg *string
	if mode == "text" {
		msg = &o.message
	}
	if err := a.paths.Publish(state, msg); err != nil {
		return die("Could not write the job's state: %v", err)
	}

	activationAttempted := false
	defer func() {
		if retErr != nil {
			if err := a.paths.Rollback(a.sys, id, activationAttempted); err != nil {
				retErr = die("%v; rollback failed, job retained for inspection: %v", retErr, err)
			}
		}
	}()

	// Unit files live on disk rather than being transient, because transient
	// units are written to /run and would be lost on reboot.
	if err := os.MkdirAll(a.paths.UnitDir, 0o777); err != nil {
		return die("Could not create %s: %v", a.paths.UnitDir, err)
	}
	service, timer := a.paths.UnitPath(id, "service"), a.paths.UnitPath(id, "timer")
	if err := job.AtomicWrite(service, []byte(serviceText), 0o644); err != nil {
		return die("Could not write %s: %v", service, err)
	}
	if err := job.AtomicWrite(timer, []byte(job.TimerUnit(id, time.Unix(runAt, 0).UTC().Format(when.Calendar)+" UTC", o.persistent)), 0o644); err != nil {
		return die("Could not write %s: %v", timer, err)
	}
	if err := a.sys.Quiet("daemon-reload"); err != nil {
		return die("systemctl daemon-reload failed: %v", err)
	}
	activationAttempted = true
	if err := a.sys.Quiet("enable", "--now", unit+".timer"); err != nil {
		return die("Could not arm %s.timer: %v", unit, err)
	}
	// Rendering may read panes again; release the reservation once arming is
	// settled, before an immediately firing worker needs to clean up.
	_ = lock.Close()
	a.receipt(sched, now)
	return nil
}

// receipt prints the send armed, or the dry run, styled when stdout is a
// terminal and as key=value lines for anything reading it.
func (a *app) receipt(s show.Scheduled, now int64) {
	if a.o.json {
		var st limit.Status
		note := ""
		if a.tgt.IsAgent() {
			st = a.limits()[a.tgt.Pane]
			switch st.State {
			case limit.Account:
				note = "account limited"
			case limit.None:
			default:
				note = limitNote(st, a.now)
			}
		}
		s.JSON(a.stdout, st, note)
		return
	}
	if a.stdoutTTY() {
		s.Styled(a.stdout, term.ColorsFor(a.stdout), now)
		return
	}
	s.Plain(a.stdout)
}

// startClock sets a.clock, unless a test has, to HERDR_SCHEDULE_NOW_EPOCH when
// that is set, else to the real time, which it stays when the variable is not
// an epoch.
func (a *app) startClock() error {
	if a.clock != nil {
		return nil
	}
	a.clock = func() int64 { return time.Now().Unix() }
	if v := os.Getenv("HERDR_SCHEDULE_NOW_EPOCH"); v != "" {
		if !digits.MatchString(v) {
			return die("HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch.")
		}
		fixed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return die("HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch in range.")
		}
		a.clock = func() int64 { return fixed }
	}
	return nil
}

func (a *app) locateHerdr() error {
	bin, err := herdr.Locate()
	if err != nil {
		return die("%s", err.Error())
	}
	a.h.Bin = bin
	return nil
}

// selfPath is this executable with symlinks resolved, which is what an armed
// timer runs.
func (a *app) selfPath() string {
	if a.self != "" {
		return a.self
	}
	p, err := os.Executable()
	if err == nil {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
	}
	a.self = p
	return p
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------- selection

func (a *app) chooseTarget() (target.Row, error) {
	o := a.o
	rows := target.Filter{
		Kind: o.kind, PanesOnly: o.panesOnly, Cwd: o.cwd,
		AllowSelf: o.allowSelf, Self: a.selfPane,
	}.Apply(a.rows)
	if len(rows) == 0 {
		return target.Row{}, die("No target matches the given selectors.")
	}

	// One match skips the picker, so this note is the only place the choice
	// is stated before the send is armed.
	if len(rows) == 1 {
		a.note(fmt.Sprintf("Only one target matched; selecting %s.", target.Summary(rows[0])))
		return rows[0], nil
	}
	rows = target.Sort(rows)
	var marks map[string]target.Mark
	if slices.ContainsFunc(rows, target.Row.IsAgent) {
		lim := a.limits()
		// Without --at the send waits for a limit to lift, so the agents held
		// by one are what it is most likely for.
		if o.at == "" {
			rows = limitedFirst(rows, lim)
		}
		marks = limitMarks(lim, a.now)
	}

	if !a.stdinTTY() {
		if o.pick == "best" {
			a.note(fmt.Sprintf("Multiple targets matched; picking %s.", target.Summary(rows[0])))
			return rows[0], nil
		}
		if a.o.json {
			return target.Row{}, fail("ambiguous", "Multiple targets matched. Narrow with --cwd/--kind/--pane, or use --pick best.")
		}
		fmt.Fprintln(a.stderr, "Multiple targets matched. Narrow with --cwd/--kind/--pane, or use --pick best.")
		target.Render(a.stderr, rows, marks, term.ColorsFor(a.stderr), a.selfPane, a.h.ProcName)
		return target.Row{}, failure{code: 1}
	}

	tty, err := a.openTTY()
	if err != nil {
		return target.Row{}, die("Could not open /dev/tty for target selection.")
	}
	defer tty.Close()
	target.Render(tty, rows, marks, term.ColorsFor(tty), a.selfPane, a.h.ProcName)
	for {
		fmt.Fprint(tty, "target [1]: ")
		choice, ok := term.ReadLine(tty)
		if !ok {
			return target.Row{}, die("Target selection cancelled: no input received.")
		}
		if choice == "" {
			choice = "1"
		}
		if n, err := strconv.Atoi(choice); err == nil && digits.MatchString(choice) && n >= 1 && n <= len(rows) {
			return rows[n-1], nil
		}
		fmt.Fprintf(tty, "Enter a number from 1 to %d.\n", len(rows))
	}
}

// ------------------------------------------------------- usage-limit timing

// limits is every agent's usage-limit state at the start of the run, keyed by
// pane. The screens are read on the first call only, so a run that never asks
// reads none. Only the visible screen is read: herdr can return a busy Claude
// agent's history empty, and a history read can take seconds.
func (a *app) limits() map[string]limit.Status {
	if a.lim == nil {
		var agents []limit.Agent
		for _, r := range a.rows {
			if r.IsAgent() {
				agents = append(agents, limit.Agent{Pane: r.Pane, Kind: r.Agent})
			}
		}
		a.lim = limit.Scan(agents, a.selfPane, a.now, func(p string) string {
			out, err := a.h.Read(p, "visible", 120)
			if err != nil {
				return ""
			}
			return out
		})
	}
	return a.lim
}

// held reports whether an agent is held by its own usage limit.
func held(st limit.Status) bool { return st.State == limit.Limited || st.State == limit.Over }

// limitedFirst moves the agents held by their own limit to the front, in
// limit.Before's order, and keeps the rest of rows as they were.
func limitedFirst(rows []target.Row, lim map[string]limit.Status) []target.Row {
	var front, rest []target.Row
	for _, r := range rows {
		if r.IsAgent() && held(lim[r.Pane]) {
			front = append(front, r)
		} else {
			rest = append(rest, r)
		}
	}
	slices.SortFunc(front, func(x, y target.Row) int {
		sx, sy := lim[x.Pane], lim[y.Pane]
		switch {
		case limit.Before(sx, sy):
			return -1
		case limit.Before(sy, sx):
			return 1
		}
		return strings.Compare(x.Pane, y.Pane)
	})
	return append(front, rest...)
}

// limitMarks is the picker's mark for each agent a usage limit holds.
func limitMarks(lim map[string]limit.Status, now int64) map[string]target.Mark {
	marks := map[string]target.Mark{}
	for pane, st := range lim {
		switch {
		case held(st):
			marks[pane] = target.Mark{Limited: true, Note: limitNote(st, now)}
		case st.State == limit.Account:
			marks[pane] = target.Mark{Note: "account limited"}
		}
	}
	return marks
}

// limitNote is how the picker describes an agent's own limit, e.g. "resets
// 16:00 (weekly)".
func limitNote(st limit.Status, now int64) string {
	var note string
	switch {
	case st.State == limit.Over && st.Reset != 0:
		note = "limit over since " + noteTime(st.Reset, now)
	case st.State == limit.Over:
		note = "limit over"
	case st.Reset != 0:
		note = "resets " + noteTime(st.Reset, now)
	default:
		note = "reset unknown"
	}
	var parts []string
	if st.Window != "" {
		parts = append(parts, st.Window)
	}
	if st.Unconfirmed {
		parts = append(parts, "unconfirmed")
	}
	if len(parts) > 0 {
		note += " (" + strings.Join(parts, ", ") + ")"
	}
	return note
}

// noteTime formats a reset as briefly as it reads unambiguously: the time
// alone today, with the weekday in the six days after, with the date beyond.
func noteTime(epoch, now int64) string {
	day := func(e int64) time.Time {
		y, m, d := time.Unix(e, 0).In(when.Loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	switch days := day(epoch).Sub(day(now)) / (24 * time.Hour); {
	case days == 0:
		return when.Format(epoch, when.Minute)
	case days > 0 && days <= 6:
		return when.Format(epoch, when.WeekdayMinute)
	default:
		return when.Format(epoch, when.MonthDayMinute)
	}
}

// warnBeforeReset warns when a text send set with --at lands before the reset
// of a limit that holds the target. An unconfirmed reset may have passed
// already, so it is not warned about.
func (a *app) warnBeforeReset(runAt int64) {
	t := a.tgt
	st := a.limits()[t.Pane]
	if st.State != limit.Limited && st.State != limit.Account || st.Reset == 0 || st.Unconfirmed || runAt >= st.Reset {
		return
	}
	reset, at := when.Format(st.Reset, when.WeekdayMinute), when.Format(runAt, when.WeekdayMinute)
	if st.State == limit.Limited {
		a.warn(fmt.Sprintf("Warning: %s in %s is usage-limited until %s; this send at %s comes before the reset.",
			t.Agent, t.Pane, reset, at))
		return
	}
	a.warn(fmt.Sprintf("Warning: %s's account looks usage-limited until %s (notice in %s); this send to %s at %s comes before the reset.",
		t.Agent, reset, st.Source, t.Pane, at))
}

// detectAt is the send time for a run without --at: the reset of the limit
// holding the target plus the margin, or the margin from now once that limit
// is over, confirmed on the terminal or taken as found under --auto.
func (a *app) detectAt(now int64) (int64, error) {
	var st limit.Status
	if a.tgt.IsAgent() {
		st = a.limits()[a.tgt.Pane]
	}
	var proposed int64
	fromNow := false
	switch {
	case st.State == limit.Over:
		proposed, fromNow = now+limitMargin, true
	case st.State != limit.None && st.Reset != 0:
		proposed = st.Reset + limitMargin
	}
	report := show.Report{
		Kind: a.tgt.Kind, Agent: a.tgt.Agent, Pane: a.tgt.Pane, Summary: target.Summary(a.tgt),
		Limit: st, Message: a.o.message, Keys: a.o.keys, Margin: limitMargin, Auto: a.o.auto, Now: now,
	}

	if a.o.auto {
		if proposed == 0 {
			return 0, die("%s\n--auto needs a notice that names its reset time; pass --at instead.", report.Plain())
		}
		if !a.o.json {
			if a.stderrTTY() {
				fmt.Fprint(a.stderr, "\n"+report.Styled(term.ColorsFor(a.stderr))+"\n")
			} else {
				fmt.Fprintln(a.stderr, report.Plain())
			}
		}
		return proposed, nil
	}
	return a.confirmAt(proposed, fromNow, report)
}

// confirmAt asks on the terminal when to send, after showing the report,
// offering def (an epoch, or 0 for no default). Enter takes the default; when
// fromNow, def is limitMargin from the start of the run, and Enter keeps it at
// least that far ahead of the moment it is pressed. Anything typed is
// resolved, shown back, and becomes the new default, so a correction is
// confirmed the same way the detected time is. Each answer is judged at the
// time it is given.
func (a *app) confirmAt(def int64, fromNow bool, report show.Report) (int64, error) {
	tty, err := a.openTTY()
	if err != nil {
		return 0, die("Could not open /dev/tty to confirm the send time.")
	}
	defer tty.Close()
	c := term.ColorsFor(tty)
	fmt.Fprint(tty, "\n"+report.Styled(c)+"\n\n")

	now := report.Now
	for {
		if def != 0 {
			fmt.Fprintf(tty, "  at [%s]: ", show.Time(def, now))
		} else {
			fmt.Fprint(tty, "  at: ")
		}
		input, ok := term.ReadLine(tty)
		if !ok {
			return 0, die("No send time given.")
		}
		now = a.clock()
		if input == "" {
			switch {
			case def == 0:
				continue
			case fromNow:
				return max(def, now+limitMargin), nil
			}
			return def, nil
		}
		epoch, parsed := when.Resolve(input, now)
		switch {
		case !parsed:
			fmt.Fprintf(tty, "    %sCould not parse '%s'. Try 00:45, 12:45 AM, Sep 27 1am, or +30 min.%s\n", c.Red, input, c.Reset)
		case epoch <= now:
			fmt.Fprintf(tty, "    %s%s has already passed.%s\n", c.Red, show.Time(epoch, now), c.Reset)
		default:
			fmt.Fprintf(tty, "    → %-11s %s\n", show.Time(epoch, now), c.Dim+"in "+when.Delay(epoch-now)+c.Reset)
			def, fromNow = epoch, false
		}
	}
}

// ------------------------------------------------------------- subcommands

func (a *app) list() {
	var jobs []show.Job
	for _, f := range a.paths.StateFiles() {
		s, err := job.Load(f)
		if err != nil {
			a.warn(fmt.Sprintf("Could not read job state %s: %v", f, err))
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		if s.ID != id {
			a.warn(fmt.Sprintf("Invalid job state %s: id does not match its filename.", f))
			continue
		}
		state := a.sys.Output("is-active", job.UnitPrefix+"-"+s.ID+".timer")
		if state == "" {
			state = "gone"
		}
		j := show.Job{ID: s.ID, At: s.At, Kind: s.Kind, Agent: s.Agent, Pane: s.PaneID, State: state, Payload: job.Preview(f)}
		if s.Mode == "keys" {
			j.Payload, j.Keys = "keys: "+s.Keys, s.Keys
		} else if b, err := os.ReadFile(strings.TrimSuffix(f, ".json") + ".msg"); err == nil {
			j.Message = string(b)
		}
		j.RunAt = s.RunAt
		if j.RunAt == 0 {
			if at, err := time.ParseInLocation(when.Calendar, s.At, when.Loc); err == nil {
				j.RunAt = at.Unix()
			}
		}
		jobs = append(jobs, j)
	}
	if a.o.json {
		show.ListJSON(a.stdout, jobs)
		return
	}
	if !a.stdoutTTY() {
		show.List(a.stdout, jobs)
		return
	}
	// A bad HERDR_SCHEDULE_NOW_EPOCH leaves the real time, which is good
	// enough to say how soon each job fires.
	_ = a.startClock()
	show.ListStyled(a.stdout, term.ColorsFor(a.stdout), jobs, a.clock())
}

func (a *app) cancel(id string) error {
	if _, err := os.Stat(a.paths.StatePath(id)); err != nil {
		return fail("not_found", "No scheduled send with id: %s", id)
	}
	if err := a.paths.Remove(a.sys, id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fail("not_found", "No scheduled send with id: %s", id)
		}
		return die("Could not cancel %s: %v", id, err)
	}
	if a.o.json {
		emit(a.stdout, struct {
			Cancelled string `json:"cancelled"`
		}{id})
		return nil
	}
	if a.stdoutTTY() {
		c := term.ColorsFor(a.stdout)
		fmt.Fprintf(a.stdout, "  %s✓%s cancelled %s\n", c.Green, c.Reset, id)
		return nil
	}
	fmt.Fprintln(a.stdout, "cancelled "+id)
	return nil
}

// showTargets lists every pane and, for an agent, the usage limit read from
// its screen. It needs no message and does not talk to systemd.
func (a *app) showTargets() error {
	if err := a.locateHerdr(); err != nil {
		return err
	}
	if err := a.startClock(); err != nil {
		return err
	}
	a.now = a.clock()
	var err error
	if a.rows, err = a.discoverTargets(); err != nil {
		return err
	}
	if len(a.rows) == 0 {
		return die("Herdr reported no panes.")
	}
	rows := target.Sort(a.rows)
	var marks map[string]target.Mark
	lim := map[string]limit.Status{}
	if slices.ContainsFunc(rows, target.Row.IsAgent) {
		lim = a.limits()
		rows = limitedFirst(rows, lim)
		marks = limitMarks(lim, a.now)
	}
	if a.o.json {
		listed := make([]show.Target, 0, len(rows))
		for _, r := range rows {
			st := lim[r.Pane]
			note := ""
			switch st.State {
			case limit.Account:
				note = "account limited"
			case limit.None:
			default:
				note = limitNote(st, a.now)
			}
			listed = append(listed, show.Target{
				Kind: r.Kind, Pane: r.Pane, Agent: r.Agent, Status: r.Status,
				Cwd: r.Cwd, Title: r.Title, Limit: st, Note: note,
			})
		}
		show.TargetsJSON(a.stdout, listed)
		return nil
	}
	target.Render(a.stdout, rows, marks, term.ColorsFor(a.stdout), a.selfPane, a.h.ProcName)
	return nil
}

// discoverTargets captures child diagnostics so JSON output is never mixed
// with a subprocess's plain text and decoding failures remain explainable.
func (a *app) discoverTargets() ([]target.Row, error) {
	var diagnostic bytes.Buffer
	prev := a.h.Stderr
	a.h.Stderr = &diagnostic
	defer func() { a.h.Stderr = prev }()
	rows, err := a.h.Targets()
	if err != nil {
		detail := strings.TrimSpace(diagnostic.String())
		if detail != "" {
			return nil, die("Could not discover Herdr targets: %v: %s", err, detail)
		}
		return nil, die("Could not discover Herdr targets: %v", err)
	}
	return rows, nil
}
