# SS-1 — Go port, limit-aware picker, polished output

## Status

P1 (Go port) is done and fast-forwarded into `ss-1`. P2–P4 are not started.
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
| P2 | Limits detected for every agent before the picker, from pane output | not started |
| P3 | Styled terminal output; `key=value` unchanged when piped | not started |
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
