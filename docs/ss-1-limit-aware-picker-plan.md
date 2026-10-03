# SS-1 — Go port, limit-aware picker, polished output

## Status

P1 (Go port) and P2 (limits before the picker) are done and fast-forwarded
into `ss-1`. P3 is being built on `ss-1-p3`; P4 is not started.
`master` still runs the zsh script; nothing in this story is live until `ss-1`
merges and `tools/install` is run in the main checkout.

## The story

`schedule-send` learns about usage limits too late: its detector runs only
after a target is picked, so a limited agent is offered as plain `idle` and is
not the default. It also prints one output for every reader. The story ports the
script to Go with tests, moves limit detection in front of the picker, and
builds each result once to render it either for a person or for a program.

## Decisions (settled 2026-10-03)

1. **Language: Go.** Rewritten as a Go binary rather than split zsh. Chosen over
   Rust; herdr is Rust, but only its CLI is used, so that gives nothing.
2. **Limited agents rank first only without `--at`.** With `--at` they are
   marked, and a send timed before a known reset is warned about.
3. **Output modes.** Styled when stdout is a terminal; today's `key=value`
   unchanged when it is not; JSON with `--json`. `--json` changes what is
   printed, never whether the command asks.
4. **Limits are read from herdr's pane output only.** Claude's session logs are
   not read, even though they hold an exact `resetsAt`.
5. **Looks** as in the walkthrough mock-up: a yellow `◷ limited` row with its
   reset, other agents of the same kind marked "account limited", and the limit
   report, time prompt, and receipt in the picker's aligned, dim style.

## Phases

| Phase | What | Status |
|---|---|---|
| P1 | Go port, identical behaviour, with tests | done |
| P2 | Limits detected for every agent before the picker, from pane output | done |
| P3 | Styled terminal output; `key=value` unchanged when piped | building |
| P4 | `--json` everywhere, a `--targets` listing, JSON errors on stderr | not started |

## P1 — Go port

### What the code check found

- **1,378 lines of zsh, no tests.** The only check so far was `zsh -n`. The
  riskiest code matches text on agents' screens: the submit gate, the
  usage-limit watch, and notice parsing.
- **Armed jobs call the script by path.** A unit's `ExecStart` is
  `/home/dev/src/schedule-send/schedule-send.zsh __fire <id>`, and the job's state
  is a JSON file in `~/.local/state/herdr-send/`. The port must keep both
  working: the state file keeps its exact schema, and `schedule-send.zsh`
  becomes a shim that runs the binary.
- **The installed command runs from the main checkout.** `~/.local/bin/
  schedule-send` points into `/home/dev/src/schedule-send`, so building there would
  change what armed jobs run. Work happens in worktrees under
  `/home/dev/src/schedule-send.worktrees/`.
- **The worker's locale is `C.UTF-8`, not none.** `systemctl --user
  show-environment` reports `LANG=C.UTF-8`, under which zsh's `[:alnum:]` keeps
  non-ASCII letters (`zażółć`). The port treats letters and digits as Unicode.
- **Time parsing stays with GNU `date -d`.** It is what accepts `+30 min` and
  `Sep 27 1am` at the prompt, and the tool is Linux-only already. Formatting
  moves into Go.
- **No `make` on this machine.** Build and check are small scripts.
- **Picker speed is not a concern.** 85 ms today, measured 2026-10-03.

### Steps

1. Toolchain and skeleton: Go 1.27.1 in `~/.local/go`, a module, `tools/check`
   (gofmt, vet, tests) → verify: `tools/check` passes.
2. Pure logic with table tests: screen text (squash, rules, prompt lines,
   composer), notice detection, time resolution and formatting, target
   filtering, ranking and picker rendering → verify: tests over screens saved
   from live panes on 2026-10-03.
3. herdr client, job store and systemd units, keeping the state schema and unit
   text → verify: a state file written by the zsh version loads; unit text
   matches.
4. CLI and scheduling flow: same flags, messages and exit codes; picker and time
   confirmation on `/dev/tty`; `--dry-run`, `--list`, `--cancel`, `--status` →
   verify: the same commands run through `master`'s zsh and the Go binary
   against a stand-in herdr give identical stdout, stderr and exit codes.
5. Worker (`__fire`): fire-time checks, keys, pane run, submit gate with its
   ladder, limit watch, cleanup, on an injected clock → verify: scenario tests,
   and one real systemd timer fired against a stand-in herdr.
6. Shim, install script, README → verify: the shim fires a zsh-written job
   under the stand-in herdr.
7. Gate, As built, fast-forward into `ss-1`.

### Not included

New behaviour of any kind: limit marking, styling and JSON are P2–P4. The
install switch happens when `ss-1` merges into `master`.

### As built

**What exists.** A Go module using only the standard library: 2,541 lines in
`cmd/schedule-send` and nine packages under `internal/` (`screen`, `notice`,
`when`, `target`, `term`, `herdr`, `job`, `cli`, `worker`), plus 1,545 lines
of tests. `schedule-send.zsh` is a 12-line shim that runs `bin/schedule-send`.
`tools/check` is the gate, `tools/install` builds and links, and
`tools/parity` is this phase's acceptance check. Stand-ins for herdr,
systemctl and journalctl are in `testdata/standin/`, and screens saved from
live panes on 2026-10-03 are in `testdata/fixtures/`.

**Measured on 2026-10-03.**

- `tools/check`: 70 tests and 4 subtests pass, none skipped.
- `tools/parity`: 78 cases (58 for the command line, 20 for the worker), each
  run through the zsh script at `db50943` and through the binary. 75 are
  identical in stdout, stderr, exit status, files written, file modes, and
  every herdr and systemctl call. The other 3 are the known differences below.
  The terminal cases run on a pseudo-terminal, so colour codes are compared
  too. A full run takes 69 s, mostly the 10 s limit watch.
- **A real systemd timer round trip.** Armed through the binary at 16:19:00
  against a stand-in herdr, on pane `wZ:p9`, which the real herdr does not
  have. It fired at 16:19:12, logged "prompted agent in wZ:p9" at 16:19:22
  after the limit watch, and removed its state, message and unit files.
- **The shim** fired a copy of the real 16:01 job's state file, as the zsh
  version wrote it, under a bare `env -i` environment.

**Deliberate differences from the zsh version.**

- A quoted `~` in `--cwd` is expanded. The zsh `${~1}` inside quotes never
  expanded, so such a selector matched nothing. Parity case `cwd-tilde-two`.
- `--cancel` or `--status` with no value prints one message and exits 1. The
  zsh printed two messages, and `--status` exited 0.
- `jq` is no longer needed.
- Failure paths where zsh died silently under `set -e` now fail with a message,
  and in the worker with a notification and the job removed. They are a herdr
  read failure while scheduling, the `--if-busy abort` status read (now
  "unknown"), a missing message file (`fire-no-message`), and a pane list herdr
  cannot return at fire time (`fire-herdr-unreadable`).
- Empty-string fields from herdr count as missing, the same as `null`.
- Waits are timed exactly. zsh timed them in whole seconds, so its 4 s grace
  window lasted somewhere between 3 and 4 s, and its 10 s limit watch made 9
  or 10 polls. Parity therefore compares a fired job's actions but not its
  polling reads.

**Found while building and left alone.**

- When arming fails at `systemctl enable`, the state file stays behind. The
  zsh did the same; mirrored.
- Codex's composer now opens with `»`, which is not a recognised prompt
  marker. Neither submit detector can see a Codex draft, so a Codex send is
  confirmed by its state change alone.
- The zsh comments said the worker runs with no locale. It runs with
  `LANG=C.UTF-8`; those comments went with the shim.

**To confirm by hand at the merge.** Run `tools/install` in the main checkout
straight after the merge, because the shim refuses to run without
`bin/schedule-send` and armed timers would fail. Then check that the picker
looks as before and that `--list` shows any armed jobs.

**What this gives later phases.**

- The picker must read each agent's `visible` screen: for a Claude agent that
  is not idle, `recent-unwrapped` can come back empty.
- Both agents print a usage gauge in their footer: Claude's `week:11% (1h58m)`
  and Codex's `5h 100% left`. That is pane output, so P2 can use it.
- Recognising Codex's `»` marker would switch its submit detectors back on. It
  changes behaviour, so it belongs in P2 or later, not in the port.
- From P2 on, a parity case whose behaviour changes on purpose moves into the
  known-differences list in `tools/parity`, with the reason.

## P2 — limits before the picker

### What the code check found

- **Screen reads are cheap; history reads are not.** Measured 2026-10-03: a
  `visible` read takes 7–9 ms per agent; `recent-unwrapped` takes 52–58 ms warm
  and once took 5.1 s cold (wB:p1). The check reads `visible` only, every agent
  in parallel, and the time prompt reuses it.
- **An old notice proposes tomorrow.** Claude's notice names only "resets 4pm".
  On the 15:16 fixture screen at 16:16, with no prompt since, today's code
  proposes Sun 16:01 and `--auto` takes it unasked. Herdr keeps no per-agent
  timestamps (`agent get` checked), so the screen is the only evidence.
- **The footer gauge bounds the reset.** The Claude statusline in use
  prints usage *left* and time to reset
  from Claude's rate-limit data: `session:N% (HH:MM)` and `week:N% (XdYh |
  XhYm | Xm)`. N is 100 − used and can go negative (`session:-1%` seen at
  17:54). It is only drawn when Claude has rate-limit data (wA:p1 had none at
  16:59). Codex prints `5h N% left · weekly N% left` (often cut off with `…`),
  with no reset time; Codex notices carry a full date anyway.
- **Codex's `»` input box breaks two things.** Codex echoes a sent prompt as
  `› resume` and draws its empty input box as `» Ask Codex to do anything`;
  only `›` was a prompt marker. A Codex agent resumed once still counted as
  limited (only the echo counted below the notice), and the worker took the
  echo for the input box, so a sent message of 8+ letters read as unsent: an
  extra Enter and a false failure. Found by reading the code against the 16:59
  screen, not reproduced. `master` has the same fault.
- **Same agent kind is not always the same account.** Two agents of the same
  kind can show different weekly gauges and resets, because each can run
  under its own account. The footer veto below covers it when a footer is
  drawn.
- **`now` is read after the target is chosen.** The check needs it first.

### Decisions (settled 2026-10-03; "implement" read as accepting the four recommendations)

1. **Old-notice rule.** A dated reset in the past means the limit is over. A
   time-only reset (`resets 4pm`) resolves to its next occurrence R. It cannot
   be further away than the agent's own gauge for that window says (now + time
   to reset + 1 min), or than 5 h for a session limit; when R is beyond that
   bound, the reset already passed and the limit is over. A gauge showing more
   than 0% left for that window also means over. With no bound and R more than
   12 h away, R is kept but marked unconfirmed, and the report says the reset
   may already have passed.
2. **"Limit over" agents rank first,** marked as such; the time prompt proposes
   now + 1 min.
3. **Account-limited agents are marked, not moved up.** An agent whose own
   gauge shows more than 0% left for that window is not marked, and is not
   given another agent's reset by the time prompt either.
4. **The Codex `»` fix is part of P2,** in the shared prompt-line rule, so the
   worker gets it too. `master` is not changed.

Assumptions: `--pick best` without `--at` takes the top row, so a limited
agent; several limited agents order by soonest reset (over first, unknown
reset last); the pane the command runs in is never read; `key=value` output is
unchanged; plain panes are never read.

### Steps

1. Screen parsing: `»` is a prompt marker; footer gauges (Claude statusline,
   Codex) parsed from the lines below the last prompt line; notices know their
   window (session, weekly, or unknown) → verify: tests on the saved screens.
2. Limit state per agent from notice, gauge and the old-notice rule → verify:
   the 15:16 screen reads limited until 16:00 at 15:22 and over at 16:16; a
   session notice 6 h out reads over; a resumed Codex agent reads not limited.
3. The parallel check before the picker; ranking without `--at`; picker marks
   as in the mock-up → verify: picker text tests; parity cases that change on
   purpose listed with reasons; check time measured on live panes, read-only.
4. The `--at` warning and the time prompt reusing the check → verify: a warning
   for `--at 15:30` against a 16:00 reset and none after; no screen read twice.
5. Worker: the shared prompt rule → verify: a Codex echo above a `»` input box
   reports a 24-character message sent, with no extra Enter.
6. Hand check: `--dry-run` against live panes (read-only); a typed, unsent
   Codex draft read back to see its marker.
7. Gate, As built, fast-forward into `ss-1`.

### Not included

Near-limit hints (a low gauge with no notice), a gauge alone marking an agent
limited, styling the report, prompt and receipt (P3), JSON (P4), and changes
to `master`.

### As built

**What exists.** A new package, `internal/limit` (308 lines, 276 of tests),
reads each agent's state from its screen: `Gauges`, `Read`, `Scan` (every
agent in parallel, with the account marks and the footer veto) and `Before`
(ranking order). `screen` accepts `»`, `notice` knows its window, and `when`
can tell a time-only reset. The command line runs one check per run, only
when its answer is used. That check feeds the picker marks and ranking,
`--pick best`, the time prompt and the `--at` warning. The README covers all
of it. The module is now 3,017 lines of code and 2,249 of tests.

**Measured on 2026-10-03.**

- `tools/check`: 94 tests and 15 subtests pass, none skipped.
- `tools/parity`: 78 cases. 68 are identical, and 10 are known differences:
  the 3 zsh bugs from P1, and 7 cases whose output changes on purpose. Each
  of the 7 is listed in `tools/parity` with its reason. Screen reads are no
  longer compared for any case; every action still is.
- **Live and read-only, at 18:38.** On the real panes, the listing and the
  terminal picker both drew `◷ limited  claude  wB:p1 … sb-42  resets 19:00
  (session)`. The time prompt proposed 19:01, the time of a send to wB:p1
  that was already armed. Each run stopped at the duplicate check, so
  nothing was armed. A run that checks one agent takes 30–40 ms, against
  10 ms for one that reads no screen. wE:p1 was the pane running the command.

**Differences from the plan.**

- The time prompt judges each answer at the moment it is given. A "1 min
  from now" default accepted late moves to a minute after Enter. Otherwise a
  slow Enter on the new over case failed with "already passed".
- Under `--auto` the unconfirmed line says to rerun with `--at '+1 min'`,
  since there is no prompt to type into.
- An unconfirmed reset gets no `--at` warning, because it may already have
  passed.
- Picker reset times are `16:00` for today, `Sun 16:00` within six days, and
  `Oct 10 00:27` beyond that.
- The clock is read before the target is chosen, so a bad
  `HERDR_SCHEDULE_NOW_EPOCH` now fails before the picker.

**Found while building and left alone.**

- **Claude's footer redraws only when Claude does.** An idle agent's
  time-to-reset counts from when it was drawn: at 18:38 wB:p1 still showed
  the afternoon's `session:-1% (01:11)`. The bound can only come out later
  than the true one. So a stale footer can delay an "over" reading but never
  produce a false one. It can also hide an "account limited" mark.
- **Relative times ignore `HERDR_SCHEDULE_NOW_EPOCH`.** `+1 min`, typed or
  given to `--at`, is resolved by date(1) from the real clock. The zsh
  version did the same.
- **No race detector.** There is no C compiler here, so `Scan` was not run
  under it.

**To confirm by hand.** The Codex draft read-back did not happen, because no
Codex agent was open. To do it: open one, type a message without sending it,
and read its screen to see that the draft sits behind `»`.

**What this gives later phases.** `limit.Status` holds everything the
report shows: state, window, reset, unconfirmed, quote and source pane. P3
can style the output from it, and P4 can print it as JSON.

## P3 — styled terminal output

### What the code check found

- **Where each output goes.** The limit report and the time prompt are
  written to `/dev/tty`, except under `--auto`, where the report goes to
  stderr. The receipt, the dry run, `--list` and `--cancel` write to stdout.
  The command can ask whether stdin is a terminal, but not stdout or stderr.
- **Nothing is built before it is printed.** Each result is written line by
  line with `Fprintf` as it is worked out. Printing one result two ways, and
  later as JSON, needs the result as a value first.
- **Parity has a strong check for this phase.** Of its 78 cases, 7 run on a
  pseudo-terminal: the picker and confirm cases. Those change on purpose. The
  other 71 are piped and must stay byte-identical.
- **The mock-up covers the report, the prompt and the receipt.** The dry run
  and `--list` are not in it.

### Decisions (settled 2026-10-03; "implement the p3" read as building to story decisions 3 and 5)

1. **Styled only where the output is going to a terminal.** Anything piped
   or redirected stays byte-identical to P2, including `--auto`'s report on
   stderr. `NO_COLOR` turns off the colour but keeps the styled layout.
2. **The report, prompt and receipt follow the mock-up.** The dry run,
   `--list` and `--cancel` use the same layout:

   ```
   target [1]:

     ◷ claude wB:p1 hit its weekly limit
       resets   Sat 16:00   in 44m
       send     "resume"    1 min after

     at [Sat 16:01]:

     ✓ scheduled "resume" for claude wB:p1
       at       Sat 16:01   in 45m
       cancel   schedule-send --cancel 20261003-160100-wBp1
   ```

3. **Left unstyled:** `--status`, errors, warnings, the usage text, and the
   picker listing printed when there is no terminal.

### Steps

1. Results built once: a `show` package holds the receipt, the dry run, the
   limit report and the job list as values, each with today's plain
   rendering → verify: tests pass; `tools/parity` unchanged (68 identical,
   10 known).
2. A styled rendering of each, chosen by where the output is going →
   verify: snapshot tests under `NO_COLOR` for every block; colour placement
   tested with marker colours; in parity, the 71 piped cases are identical
   and the 7 terminal cases are listed as known, with reasons.
3. README → verify: its examples match the snapshot tests.
4. Hand check: a dry run with colour on a pseudo-terminal against the live
   panes, read back as text → verify: you look at it.
5. Gate, As built, fast-forward into `ss-1`.

### Not included

`--json` (P4), styling errors, warnings and `--status`, and an arrow-key
picker.
