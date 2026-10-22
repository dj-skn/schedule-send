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
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dj-skn/schedule-send/internal/herdr"
	"github.com/dj-skn/schedule-send/internal/installpath"
	"github.com/dj-skn/schedule-send/internal/job"
	"github.com/dj-skn/schedule-send/internal/limit"
	"github.com/dj-skn/schedule-send/internal/show"
	"github.com/dj-skn/schedule-send/internal/target"
	"github.com/dj-skn/schedule-send/internal/term"
	"github.com/dj-skn/schedule-send/internal/when"
	"github.com/dj-skn/schedule-send/internal/worker"
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

// installHint is how to get a build that can arm jobs.
const installHint = "Install it again with Homebrew, the install script, or tools/install from a checkout, then run the installed schedule-send."

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
	sys            job.Scheduler
	h              herdr.Client
	selfPane       string                 // HERDR_PANE_ID
	installedPath  func() (string, error) // private injection for isolated tests
	exit           func(code int)         // os.Exit; tests catch it
	statusTimeouts statusTimeouts         // private deadlines for isolated tests

	rows []target.Row
	tgt  target.Row
	now  int64 // when the run started; screens are judged at this moment

	lim map[string]limit.Status // set by limits
	ask *asker                  // set by asking, at the first question
}

// asker is the terminal a run asks its questions on, drawn as a rail. It is
// opened at the first question and kept until the run ends, so every answer
// comes from one stream, as typed, and the rail ends with how the run came
// out.
type asker struct {
	tty  io.ReadWriteCloser
	c    term.Colors
	rail show.Rail
	stop func() // hands Ctrl-C back; nil once it has
}

// say writes lines on the terminal.
func (q *asker) say(lines ...string) {
	for _, l := range lines {
		fmt.Fprintln(q.tty, l)
	}
}

// answer shows the place to type, and reads what was typed there. ok is false
// at end of input.
func (q *asker) answer() (string, bool) {
	fmt.Fprint(q.tty, q.rail.Input())
	return term.ReadLine(q.tty)
}

// asking is the run's terminal for questions, opened and given the rail's top
// the first time.
func (a *app) asking() (*asker, error) {
	if a.ask != nil {
		return a.ask, nil
	}
	tty, err := a.openTTY()
	if err != nil {
		return nil, err
	}
	c := term.ColorsFor(tty)
	a.ask = &asker{tty: tty, c: c, rail: show.NewRail(c)}
	a.ask.say("", a.ask.rail.Top(a.o.dryRun), a.ask.rail.Gap())
	a.ask.stop = a.cancelOnInterrupt(a.ask)
	return a.ask, nil
}

// cancelOnInterrupt makes Ctrl-C while the run asks end the rail saying
// nothing was scheduled, which holds because nothing is written before the
// last answer. The function it returns hands Ctrl-C back.
func (a *app) cancelOnInterrupt(q *asker) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			q.say("", q.rail.Cancelled())
			a.exit(130)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// doneAsking hands Ctrl-C back once the last answer is in, before anything
// is written, so an interrupted write ends the way it always has.
func (a *app) doneAsking() {
	if a.ask != nil && a.ask.stop != nil {
		a.ask.stop()
		a.ask.stop = nil
	}
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
		exit:     os.Exit,
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
	if a.ask != nil {
		a.doneAsking()
		defer a.ask.tty.Close()
	}
	var f failure
	switch {
	case err == nil:
		return 0
	case errors.As(err, &f):
		if f.code == 0 || f.msg == "" {
			return f.code
		}
		a.failed(f)
		return f.code
	default:
		a.failed(failure{msg: err.Error(), code: 1})
		return 1
	}
}

// failed says why the run stopped: as JSON under --json, as the rail's end
// once a question was asked, and otherwise on stderr.
func (a *app) failed(f failure) {
	switch {
	case a.o.json:
		a.writeError(f)
	case a.ask != nil:
		a.ask.say(a.ask.rail.Fail(f.msg))
	default:
		fmt.Fprintln(a.stderr, f.msg)
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
// so stderr stays something a script can read. A warning given once a question
// was asked goes on the rail instead, among the steps it is about.
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
	if a.ask != nil {
		a.ask.say(a.ask.rail.Warning(msg))
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
		case "--json", "--targets", "--list", "--cancel", "--status", "--help", "-h", "--version":
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
		case "--version":
			err = operation(flag)
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
	case "--version":
		if a.o.json {
			emit(a.stdout, struct {
				Version string `json:"version"`
			}{Version})
			return nil
		}
		fmt.Fprintln(a.stdout, "schedule-send", Version)
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

	// A dry run previews the same executable arming would record, and says
	// when this build could not arm at all.
	resolve := a.installedPath
	if resolve == nil {
		resolve = installpath.Resolve
	}
	installed, installErr := resolve()
	if !o.dryRun && installErr != nil {
		return die("Cannot arm a job: %v. %s", installErr, installHint)
	}
	if err := a.sys.Available(!o.dryRun); err != nil {
		return die("%v", err)
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
	// Asked on a terminal, the rail asks for the message a shell pane needs,
	// and its report says when an agent is blocked.
	ask := detect && !o.auto
	if o.defaulted && !t.IsAgent() && !ask {
		return die("Pane %s is a plain shell, where the default '%s' would run as a command. Pass --message.", t.Pane, resumeMessage)
	}
	if t.IsAgent() && t.Status == "blocked" && o.keys == "" && !ask {
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
	a.doneAsking()
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
		DryRun: o.dryRun, ID: id, Unit: a.sys.Name(id),
		Kind: t.Kind, Pane: t.Pane, Agent: t.Agent, Status: t.Status, Cwd: t.Cwd,
		At: calendar, RunAt: runAt, Delay: delay,
		Mode: mode, Payload: payload, Message: o.message, Keys: o.keys,
		Persistent: o.persistent, IfBusy: o.ifBusy, Recover: o.recover, Grace: o.grace,
		UnitFiles: a.sys.Files(a.paths, id, o.persistent),
	}
	// The program and arguments the timer runs; the unit wraps them in
	// /usr/bin/env with systemd escaping.
	if installErr == nil {
		sched.WouldRun = installed + " __fire " + id
	}

	if o.dryRun {
		if installErr != nil {
			a.warn(fmt.Sprintf("Warning: this build cannot arm jobs (%v), so a real run would be refused. %s", installErr, installHint))
		}
		a.receipt(sched, now)
		return nil
	}

	trigger, err := a.sys.Render(t.Pane, calendar, installed, id, runAt, o.persistent)
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

	if activationAttempted, err = a.sys.Arm(a.paths, trigger); err != nil {
		return die("%v", err)
	}
	// Rendering may read panes again; release the reservation once arming is
	// settled, before an immediately firing worker needs to clean up.
	_ = lock.Close()
	a.receipt(sched, now)
	return nil
}

// receipt prints the send armed, or the dry run, styled when stdout is a
// terminal and as key=value lines for anything reading it. After questions it
// is the rail's end, and stdout gets it only when that is not the same
// terminal.
func (a *app) receipt(s show.Scheduled, now int64) {
	if a.ask != nil {
		a.ask.say(s.RailEnd(a.ask.c, now))
		if a.stdoutTTY() && !a.o.json {
			return
		}
	}
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

	// The picker is the rail's first step, its list beside the rail.
	q, err := a.asking()
	if err != nil {
		return target.Row{}, die("Could not open /dev/tty for target selection.")
	}
	var list bytes.Buffer
	target.Render(&list, rows, marks, q.c, a.selfPane, a.h.ProcName)
	q.say(q.rail.Step("Which agent or pane?"))
	for _, line := range strings.Split(strings.Trim(list.String(), "\n"), "\n") {
		q.say(q.rail.Line(line))
	}
	q.say(q.rail.Gap(), q.rail.Hint("Enter takes 1, or type its number"))
	for {
		choice, ok := q.answer()
		if !ok {
			return target.Row{}, die("Target selection cancelled: no input received.")
		}
		if choice == "" {
			choice = "1"
		}
		if n, err := strconv.Atoi(choice); err == nil && digits.MatchString(choice) && n >= 1 && n <= len(rows) {
			q.say(q.rail.Gap())
			return rows[n-1], nil
		}
		q.say(q.rail.Error(fmt.Sprintf("Enter a number from 1 to %d.", len(rows))))
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
		Kind: a.tgt.Kind, Agent: a.tgt.Agent, Pane: a.tgt.Pane, Status: a.tgt.Status, Cwd: a.tgt.Cwd,
		Summary: target.Summary(a.tgt), Limit: st, Message: a.o.message, Keys: a.o.keys,
		Margin: limitMargin, Auto: a.o.auto, Now: now,
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
	q, err := a.asking()
	if err != nil {
		return 0, die("Could not open /dev/tty to confirm the send time.")
	}
	for _, line := range report.RailLines(q.c) {
		q.say(q.rail.Line(line))
	}
	q.say(q.rail.Gap())
	if a.o.defaulted {
		if err := a.askMessage(q, a.tgt.IsAgent()); err != nil {
			return 0, err
		}
	}
	return a.confirmAt(q, proposed, fromNow, now)
}

// askMessage asks what to send when no message was given. Enter keeps
// "resume" on an agent; a shell pane, where the text runs as a command, has
// to be told.
func (a *app) askMessage(q *asker, agent bool) error {
	hint := `Enter keeps "` + resumeMessage + `"`
	if !agent {
		hint = "It's a shell, so this runs as a command"
	}
	q.say(q.rail.Step("What should it send?"), q.rail.Hint(hint))
	for {
		input, ok := q.answer()
		if !ok {
			return die("No message given.")
		}
		switch {
		case strings.TrimSpace(input) != "":
			a.o.message = input
		case agent:
			a.o.message = resumeMessage
		default:
			continue
		}
		q.say(q.rail.Gap())
		return nil
	}
}

// confirmAt asks on the rail when to send, offering def (an epoch, or 0 for
// no default). Enter takes the default; when fromNow, def is limitMargin from
// the start of the run, and Enter keeps it at least that far ahead of the
// moment it is pressed. Anything typed is resolved, shown back, and becomes
// the new default, so a correction is confirmed the same way the detected time
// is. Each answer is judged at the time it is given.
func (a *app) confirmAt(q *asker, def int64, fromNow bool, now int64) (int64, error) {
	q.say(q.rail.Step("When?"), q.rail.Hint(timeHint(def, fromNow, now)))
	for {
		input, ok := q.answer()
		if !ok {
			return 0, die("No send time given.")
		}
		now = a.clock()
		if input == "" {
			switch {
			case def == 0:
				continue
			case fromNow:
				def = max(def, now+limitMargin)
			}
			q.say(q.rail.Gap())
			return def, nil
		}
		epoch, parsed := when.Resolve(input, now)
		switch {
		case !parsed:
			q.say(q.rail.Error(fmt.Sprintf("Could not read '%s'. Try 18:30, 6:30 PM, Sep 27 1am or +30 min.", input)))
		case epoch <= now:
			q.say(q.rail.Error(show.Time(epoch, now) + " has already passed."))
		default:
			q.say(q.rail.Resolved(show.Time(epoch, now), when.Delay(epoch-now)))
			def, fromNow = epoch, false
		}
	}
}

// timeHint says what the time question takes: the time Enter keeps and why,
// or, with none to keep, the ways a time can be typed.
func timeHint(def int64, fromNow bool, now int64) string {
	mins := limitMargin / 60
	switch {
	case def == 0:
		return "Like 18:30, 6:30 PM, Sep 27 1am or +30 min"
	case fromNow:
		return fmt.Sprintf("Enter keeps %s, %d min from now", show.Time(def, now), mins)
	}
	return fmt.Sprintf("Enter keeps %s, %d min after the reset", show.Time(def, now), mins)
}

// ------------------------------------------------------------- subcommands

func (a *app) list() {
	var jobs []show.Job
	for _, f := range a.paths.StateFiles() {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		s, err := job.Load(f)
		if err != nil {
			a.warn(fmt.Sprintf("Could not read job state %s: %v", f, err))
		} else if s.ID != id {
			err = errors.New("id does not match its filename")
			a.warn(fmt.Sprintf("Invalid job state %s: id does not match its filename.", f))
		}
		j := show.Job{ID: id, At: s.At, Kind: s.Kind, Agent: s.Agent, Pane: s.PaneID, Payload: job.Preview(f)}
		if err != nil {
			// Listed rather than skipped: its timer will refuse it, and it
			// stays until cancelled.
			j.State, j.Error = "invalid", err.Error()
		} else if j.State = a.sys.Armed(id); j.State == "" {
			j.State = "gone"
		}
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
