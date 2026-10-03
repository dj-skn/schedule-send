# SS-1 — Go port, limit-aware picker, polished output

## Status

P1 (Go port) is being built on `ss-1-p1`. P2–P4 are not started. `master`
still runs the zsh script; nothing in this story is live until `ss-1` merges.

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
| P1 | Go port, identical behaviour, with tests | building |
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
