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
	"strconv"
	"strings"
	"time"

	"schedule-send/internal/herdr"
	"schedule-send/internal/job"
	"schedule-send/internal/notice"
	"schedule-send/internal/target"
	"schedule-send/internal/term"
	"schedule-send/internal/when"
	"schedule-send/internal/worker"
)

// failure ends the command: msg goes to stderr, code is the exit status.
type failure struct {
	msg  string
	code int
}

func (f failure) Error() string { return f.msg }

func die(format string, a ...any) error { return failure{fmt.Sprintf(format, a...), 1} }

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
}

type app struct {
	o              options
	stdout, stderr io.Writer
	stdinTTY       func() bool
	openTTY        func() (io.ReadWriteCloser, error)
	paths          job.Paths
	sys            job.Systemctl
	h              herdr.Client
	selfPane       string // HERDR_PANE_ID
	self           string // this executable, which the timer runs

	rows []target.Row
	tgt  target.Row

	// The usage-limit notice behind a detected time.
	lnTime, lnQuote, limitSource string
	limitReset                   int64 // 0 when the notice names no readable time
}

// Main runs the command and returns its exit status.
func Main(args []string) int {
	a := &app{
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		stdinTTY: func() bool { return term.IsTerminal(os.Stdin.Fd()) },
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
	err := a.run(args)
	var f failure
	switch {
	case err == nil:
		return 0
	case errors.As(err, &f):
		if f.msg != "" {
			fmt.Fprintln(a.stderr, f.msg)
		}
		return f.code
	default:
		fmt.Fprintln(a.stderr, err)
		return 1
	}
}

// errDone ends a run that has already printed its result.
var errDone = failure{code: 0}

func (a *app) parse(args []string) error {
	o := &a.o
	o.pick, o.ifBusy, o.recover, o.grace = "unique", "wait", "keys", "4"
	for i := 0; i < len(args); {
		flag := args[i]
		value := func() (string, error) {
			v := ""
			if i+1 < len(args) {
				v = args[i+1]
			}
			if v == "" {
				return "", die("Missing value for %s.", flag)
			}
			if strings.HasPrefix(v, "--") {
				return "", die("Missing value for %s (got the flag '%s').", flag, v)
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
		case "--list":
			if err := a.locateHerdr(); err != nil {
				return err
			}
			a.list()
			return errDone
		case "--cancel":
			id, err := value()
			if err != nil {
				return err
			}
			if err := a.cancel(id); err != nil {
				return err
			}
			return errDone
		case "--status":
			id, err := value()
			if err != nil {
				return err
			}
			a.status(id)
			return errDone
		case "-h", "--help":
			fmt.Fprint(a.stdout, usage)
			return errDone
		default:
			fmt.Fprintln(a.stderr, "Unknown argument: "+flag)
			fmt.Fprint(a.stderr, usage)
			return failure{code: 2}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

var digits = regexp.MustCompile(`^[0-9]+$`)

func (a *app) run(args []string) error {
	if err := a.parse(args); err != nil {
		return err
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

	a.h.Stderr = a.stderr
	a.rows, err = a.h.Targets()
	if err != nil {
		return failure{code: 1}
	}
	if len(a.rows) == 0 {
		return die("Herdr reported no panes.")
	}

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
		fmt.Fprintf(a.stderr, "Warning: agent in %s is blocked on a prompt right now.\n", t.Pane)
	}

	nowOverride := os.Getenv("HERDR_SCHEDULE_NOW_EPOCH")
	var now int64
	if nowOverride != "" {
		if !digits.MatchString(nowOverride) {
			return die("HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch.")
		}
		now, _ = strconv.ParseInt(nowOverride, 10, 64)
	} else {
		now = time.Now().Unix()
	}

	var runAt int64
	if detect {
		if runAt, err = a.detectAt(now); err != nil {
			return err
		}
		// A confirmation can sit unanswered for a while, so "already passed"
		// and the delay are judged from the moment it was answered.
		if nowOverride == "" {
			now = time.Now().Unix()
		}
	} else {
		var ok bool
		if runAt, ok = when.Resolve(o.at, now); !ok {
			return die("Could not parse --at '%s'. Use HH:MM[:SS], H[:MM]am/pm, or YYYY-MM-DD HH:MM:SS.", o.at)
		}
	}
	if runAt <= now {
		return die("Target time has already passed: %s", when.Format(runAt, when.Calendar))
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

	if o.dryRun {
		w := a.stdout
		fmt.Fprintf(w, "target=%s %s (%s, %s)\n", t.Kind, t.Pane, t.Agent, t.Status)
		fmt.Fprintf(w, "cwd=%s\n", t.Cwd)
		fmt.Fprintf(w, "at=%s\n", calendar)
		fmt.Fprintf(w, "run_at_epoch=%d\n", runAt)
		fmt.Fprintf(w, "delay_seconds=%d\n", delay)
		fmt.Fprintf(w, "mode=%s\n", mode)
		fmt.Fprintf(w, "payload=%s\n", payload)
		fmt.Fprintf(w, "id=%s\n", id)
		fmt.Fprintf(w, "unit=%s.timer\n", unit)
		fmt.Fprintf(w, "persistent=%d\n", b2i(o.persistent))
		fmt.Fprintf(w, "if_busy=%s\n", o.ifBusy)
		fmt.Fprintf(w, "recover=%s\n", o.recover)
		fmt.Fprintf(w, "submit_grace=%s\n", o.grace)
		fmt.Fprintf(w, "unit_files=%s/%s.{service,timer}\n", a.paths.UnitDir, unit)
		fmt.Fprintf(w, "would_run=%s __fire %s\n", a.selfPath(), id)
		return nil
	}

	a.paths.Sweep(a.sys)
	state := job.State{
		ID: id, Kind: t.Kind, PaneID: t.Pane, Agent: t.Agent, Cwd: t.Cwd, At: calendar,
		Mode: mode, Keys: o.keys, IfBusy: o.ifBusy, Force: b2i(o.force), Quiet: b2i(o.quiet),
		Recover: o.recover, Grace: grace, HerdrBin: a.h.Bin, Unit: unit,
	}
	var msg *string
	if mode == "text" {
		msg = &o.message
	}
	if err := a.paths.Write(state, msg); err != nil {
		return die("Could not write the job's state: %v", err)
	}

	// Unit files live on disk rather than being transient, because transient
	// units are written to /run and would be lost on reboot.
	if err := os.MkdirAll(a.paths.UnitDir, 0o777); err != nil {
		return die("Could not create %s: %v", a.paths.UnitDir, err)
	}
	service, timer := a.paths.UnitPath(id, "service"), a.paths.UnitPath(id, "timer")
	if err := os.WriteFile(service, []byte(job.ServiceUnit(t.Pane, calendar, a.selfPath(), id)), 0o666); err != nil {
		return die("Could not write %s: %v", service, err)
	}
	if err := os.WriteFile(timer, []byte(job.TimerUnit(id, calendar, o.persistent)), 0o666); err != nil {
		return die("Could not write %s: %v", timer, err)
	}
	if err := a.sys.Through(a.stdout, a.stderr, "daemon-reload"); err != nil {
		return die("systemctl daemon-reload failed.")
	}
	if err := a.sys.Quiet("enable", "--now", unit+".timer"); err != nil {
		os.Remove(service)
		os.Remove(timer)
		_ = a.sys.Quiet("daemon-reload")
		return die("Could not arm %s.timer.", unit)
	}

	w := a.stdout
	fmt.Fprintf(w, "id=%s\n", id)
	fmt.Fprintf(w, "unit=%s.timer\n", unit)
	fmt.Fprintf(w, "target=%s %s (%s, %s)\n", t.Kind, t.Pane, t.Agent, t.Status)
	fmt.Fprintf(w, "at=%s\n", calendar)
	fmt.Fprintf(w, "delay_seconds=%d\n", delay)
	fmt.Fprintf(w, "payload=%s\n", payload)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "cancel with: schedule-send --cancel %s\n", id)
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
		fmt.Fprintf(a.stderr, "Only one target matched; selecting %s.\n", target.Summary(rows[0]))
		return rows[0], nil
	}
	rows = target.Sort(rows)

	if !a.stdinTTY() {
		if o.pick == "best" {
			fmt.Fprintf(a.stderr, "Multiple targets matched; picking %s.\n", target.Summary(rows[0]))
			return rows[0], nil
		}
		fmt.Fprintln(a.stderr, "Multiple targets matched. Narrow with --cwd/--kind/--pane, or use --pick best.")
		target.Render(a.stderr, rows, term.ColorsFor(a.stderr), a.selfPane, a.h.ProcName)
		return target.Row{}, failure{code: 1}
	}

	tty, err := a.openTTY()
	if err != nil {
		return target.Row{}, die("Could not open /dev/tty for target selection.")
	}
	defer tty.Close()
	target.Render(tty, rows, term.ColorsFor(tty), a.selfPane, a.h.ProcName)
	for {
		fmt.Fprint(tty, "target [1]: ")
		choice, ok := term.ReadLine(tty)
		if !ok || choice == "" {
			choice = "1"
		}
		if n, err := strconv.Atoi(choice); err == nil && digits.MatchString(choice) && n >= 1 && n <= len(rows) {
			return rows[n-1], nil
		}
		fmt.Fprintf(tty, "Enter a number from 1 to %d.\n", len(rows))
	}
}

// ------------------------------------------------------- usage-limit timing

// limitNotice reads the pane's current usage-limit notice into lnTime and
// lnQuote. Claude draws on the alternate screen, whose rows never reach the
// host scrollback, and herdr can recover that history only from an idle agent,
// so a recent read can come back empty; the visible screen, which still holds
// the latest exchange, stands in for it.
func (a *app) limitNotice(pane string) bool {
	a.lnTime, a.lnQuote = "", ""
	out := a.h.Read(pane, "recent-unwrapped", 120)
	if strings.TrimSpace(out) == "" {
		out = a.h.Read(pane, "visible", 120)
	}
	n, ok := notice.Current(notice.NewTranscript(out))
	if ok {
		a.lnTime, a.lnQuote = n.Time, n.Quote
	}
	return ok
}

// findLimit looks for the target's reset: its own notice first, then another
// agent of the same kind, since the limit belongs to the account rather than
// the pane. A notice whose reset already passed is old scrollback and is
// skipped. The pane this command runs in is never read.
func (a *app) findLimit(now int64) bool {
	t := a.tgt
	panes := []string{t.Pane}
	for _, r := range a.rows {
		if r.IsAgent() && r.Agent == t.Agent && r.Pane != t.Pane && r.Pane != a.selfPane {
			panes = append(panes, r.Pane)
		}
	}
	a.limitSource, a.limitReset = "", 0
	for _, p := range panes {
		if !a.limitNotice(p) {
			continue
		}
		var epoch int64
		if a.lnTime != "" {
			if e, ok := when.Resolve(a.lnTime, now); ok {
				epoch = e
			}
		}
		if epoch != 0 && epoch <= now {
			continue
		}
		a.limitSource, a.limitReset = p, epoch
		return true
	}
	return false
}

// limitReport says what was found, for the confirmation prompt or --auto's
// note on stderr.
func (a *app) limitReport(now int64) string {
	t := a.tgt
	if a.limitSource == "" {
		return "No usage-limit notice found for " + target.Summary(t) + "."
	}
	what := `"` + firstLine(a.o.message) + `"`
	if a.o.keys != "" {
		what = "keys: " + a.o.keys
	}
	var b strings.Builder
	if a.limitSource == t.Pane {
		fmt.Fprintf(&b, "Usage limit in %s %s:\n", t.Agent, t.Pane)
	} else {
		fmt.Fprintf(&b, "Usage limit in %s %s (per account; %s shows none):\n", t.Agent, a.limitSource, t.Pane)
	}
	fmt.Fprintf(&b, "  %s\n", a.lnQuote)
	if a.limitReset != 0 {
		fmt.Fprintf(&b, "Resets %s (in %s); sending %s %d min after.",
			when.Format(a.limitReset, when.WeekdayMinute), when.Delay(a.limitReset-now), what, limitMargin/60)
	} else {
		fmt.Fprintf(&b, "It names no reset time that can be read; sending %s at the time given.", what)
	}
	return b.String()
}

// detectAt is the send time for a run without --at: the target's usage-limit
// reset plus the margin, confirmed on the terminal, or taken as found under
// --auto.
func (a *app) detectAt(now int64) (int64, error) {
	a.limitSource, a.limitReset = "", 0
	if a.tgt.IsAgent() {
		a.findLimit(now)
	}
	var proposed int64
	if a.limitReset != 0 {
		proposed = a.limitReset + limitMargin
	}
	report := a.limitReport(now)

	if a.o.auto {
		if proposed == 0 {
			return 0, die("%s\n--auto needs a notice that names its reset time; pass --at instead.", report)
		}
		fmt.Fprintln(a.stderr, report)
		return proposed, nil
	}
	return a.confirmAt(proposed, now, report)
}

// confirmAt asks on the terminal when to send, after showing the report,
// offering def (an epoch, or 0 for no default). Enter takes the default.
// Anything typed is resolved, shown back, and becomes the new default, so a
// correction is confirmed the same way the detected time is.
func (a *app) confirmAt(def, now int64, report string) (int64, error) {
	tty, err := a.openTTY()
	if err != nil {
		return 0, die("Could not open /dev/tty to confirm the send time.")
	}
	defer tty.Close()
	fmt.Fprintln(tty)
	fmt.Fprintln(tty, report)
	fmt.Fprintln(tty)

	for {
		if def != 0 {
			fmt.Fprintf(tty, "at [%s]: ", when.Format(def, when.DayMinute))
		} else {
			fmt.Fprint(tty, "at: ")
		}
		input, ok := term.ReadLine(tty)
		if !ok {
			return 0, die("No send time given.")
		}
		if input == "" {
			if def != 0 {
				return def, nil
			}
			continue
		}
		epoch, parsed := when.Resolve(input, now)
		switch {
		case !parsed:
			fmt.Fprintf(tty, "Could not parse '%s'. Try 00:45, 12:45 AM, Sep 27 1am, or +30 min.\n", input)
		case epoch <= now:
			fmt.Fprintf(tty, "%s has already passed.\n", when.Format(epoch, when.DayMinute))
		default:
			fmt.Fprintf(tty, "  → %s (in %s)\n", when.Format(epoch, when.WeekdayFull), when.Delay(epoch-now))
			def = epoch
		}
	}
}

// ------------------------------------------------------------- subcommands

func (a *app) list() {
	files := a.paths.StateFiles()
	if len(files) == 0 {
		fmt.Fprintln(a.stdout, "No scheduled sends.")
		return
	}
	fmt.Fprintf(a.stdout, "%-24s %-20s %-10s %-9s %s\n", "ID", "AT", "TARGET", "STATE", "PAYLOAD")
	for _, f := range files {
		s, _ := job.Load(f)
		state := a.sys.Output("is-active", job.UnitPrefix+"-"+s.ID+".timer")
		if state == "" {
			state = "gone"
		}
		msg := job.Preview(f)
		if s.Mode == "keys" {
			msg = "keys: " + s.Keys
		}
		fmt.Fprintf(a.stdout, "%-24s %-20s %-10s %-9s %s\n", s.ID, s.At, s.PaneID, state, msg)
	}
}

func (a *app) cancel(id string) error {
	if _, err := os.Stat(a.paths.StatePath(id)); err != nil {
		return die("No scheduled send with id: %s", id)
	}
	a.paths.Remove(a.sys, id)
	fmt.Fprintln(a.stdout, "cancelled "+id)
	return nil
}

func (a *app) status(id string) {
	unit := job.UnitPrefix + "-" + id
	if b, err := os.ReadFile(a.paths.StatePath(id)); err == nil {
		var out bytes.Buffer
		if json.Indent(&out, bytes.TrimSpace(b), "", "  ") == nil {
			out.WriteByte('\n')
			a.stdout.Write(out.Bytes())
		} else {
			a.stdout.Write(b)
		}
	} else {
		fmt.Fprintf(a.stderr, "No state file for %s (job may have already run).\n", id)
	}
	fmt.Fprintln(a.stdout)
	_ = a.sys.Through(a.stdout, nil, "status", unit+".timer", "--no-pager")
	fmt.Fprintln(a.stdout)
	cmd := exec.Command("journalctl", "--user", "-u", unit+".service", "--no-pager", "-n", "40")
	cmd.Stdout = a.stdout
	_ = cmd.Run()
}
