# Herdr Scheduled Send

Schedule text or key presses into a running Herdr agent or pane. Linux only —
this is the VPS build and it depends on GNU `date` and `systemd --user`.

The delayed job is owned by an on-disk systemd user timer, so it survives the
launching shell, a logout, and a reboot. Targets are re-resolved at fire time,
so a pane that changed identity in the meantime is never written to.

Leave out the time and it is read from the agent's usage-limit notice instead,
so an agent that ran out of usage can be told to `resume` the minute its limit
resets — see [Resuming After a Usage Limit](#resuming-after-a-usage-limit).

## Install

Written in Go; building it needs Go 1.27 or newer and nothing else.

```zsh
cd ~/src/schedule-send
tools/install
hash -r
```

That builds `bin/schedule-send` and links it into `~/.local/bin`. Verify:

```zsh
command -v schedule-send
schedule-send --help
```

Run `tools/install` again after every pull or edit: armed timers run
`bin/schedule-send`, so they pick up a rebuild immediately, including jobs
that are already scheduled. Timers armed by the old zsh version run
`schedule-send.zsh`, which now hands over to the same binary.

Requires `systemctl --user` and `herdr` on `PATH`. Lingering must be enabled
for reboot survival (`loginctl show-user "$USER" -p Linger`).

Before committing a change, run `tools/check`: formatting, vet, and the tests,
which drive herdr and systemctl through stand-ins and never touch a real pane.

## Basic Usage

```zsh
schedule-send --at '15:50' --message 'resume, then commit when done'
```

With no selector this shows a picker. When exactly one target matches — a
selector that narrows to one, or a single agent being the only thing running —
it is taken without a picker and named on stderr:

```
Only one target matched; selecting codex in wV:p1 (idle, ~/projects/shop-backend).
```

On a terminal the receipt is styled:

```
  ✓ scheduled "resume, then commit when done" for claude wB:p1
    at       Sat 15:50   in 12m
    cancel   schedule-send --cancel 20261003-155000-wBp1
```

Piped or redirected, stdout stays `key=value`, which is what a script should
read:

```
id=20261003-155000-wBp1
unit=herdr-send-20261003-155000-wBp1.timer
target=agent wB:p1 (claude, idle)
at=2026-10-03 15:50:00
delay_seconds=720
payload=resume, then commit when done

cancel with: schedule-send --cancel 20261003-155000-wBp1
```

A script can rely on that, for instance to send a file:

```zsh
schedule-send --at '03:30' --cwd ~/projects/website --message-file prompt.txt
```

Time-only values use local time, in 24-hour (`15:50`, `03:30:15`) or 12-hour
(`12:27 AM`, `3pm`) form. If the time already passed today it schedules for
tomorrow, re-parsed against tomorrow's calendar date so a DST transition
between the two days cannot shift the result by an hour.

## Resuming After a Usage Limit

Leave out `--at` and the time comes from the agent's own usage-limit notice:

```zsh
schedule-send --cwd ~/projects/shop-backend
```

```
  ◷ codex w9:p1 hit a usage limit
    resets   Sun 00:27   in 37m
    send     "resume"    1 min after

  at [Sun 00:28]:
```

Enter takes the proposed time. Anything typed instead — `00:45`, `12:45 AM`,
`Sep 27 1am`, `+30 min` — is shown back with how long away it is, and becomes
the new default, so a correction is confirmed the same way:

```
    → Sat 16:30   in 1h 8m
```

With no `--message` the payload is `resume`; pass one to send something else.

The notice names its reset to the minute and the reset lands somewhere inside
it (one reading 12:27 AM was for 00:27:52), so the proposal is one minute
later: the first moment certainly past it.

What is recognised:

| Agent | Notice | Time forms |
| --- | --- | --- |
| Codex | `■ You’ve hit your usage limit … try again at …` | `Sep 27th, 2026 12:27 AM` |
| Claude | `You've hit your session limit · resets …` | `3pm`, `3:30pm`, `Sep 28, 3pm`, each optionally `(Europe/Warsaw)` |

A notice that names no time (`try again later`, `Usage limit reached`) is
still shown, and the prompt then has no default. Only the newest notice
counts, and it is ignored once a prompt has gone through below it.

These forms are accepted by `--at` too, so a time copied out of a notice works
as written.

Every agent's visible screen is read once, before the picker, and the time
prompt reuses that read; the pane you run the command in is never read. If the
target shows no notice of its own, another agent of the same kind lends its
reset, since the limit belongs to the account, and the report says which pane
it came from. It does not when the target's own footer shows usage left in that
window (`week:11% (1h58m)`), which means a different account.

A notice stays on screen after its limit lifts, until the agent is prompted
again. Once its reset has passed, or the agent's own footer shows usage left,
the limit is over and the proposal is one minute from now:

```
  ✓ claude wB:p1's weekly limit is over
    reset    Sat 16:00   passed
    send     "resume"    1 min from now
```

Claude's short form, `resets 3pm`, carries no date. It is taken as the next
3pm, unless the footer's time to reset (or a session limit's five hours) says
the reset cannot be that far off; then it was the last 3pm and the limit is
over. With no footer to go by and the next 3pm more than 12 hours away, the
report adds a line, since the notice may be yesterday's (under `--auto`, which
has no prompt, it says to rerun with `--at '+1 min'`):

```
    note     the notice names no day; if it was shown before Sat 15:00, the limit is already over: type +1 min
```

With `--at`, a message timed before a known reset of a limit holding the target
is still scheduled, with a warning on stderr:

```
Warning: claude in wB:p1 is usage-limited until Sat 16:00; this send at Sat 15:30 comes before the reset.
```

With no terminal there is nobody to confirm, so leaving out `--at` fails unless
`--auto` is passed. `--auto` takes the proposed time unconfirmed, prints the
report on stderr, and fails when there is none:

```zsh
schedule-send --cwd ~/projects/shop-backend --auto
```

A plain shell pane has no notice to read, and the default `resume` would run as
a command there, so it needs `--at` and `--message`.

## Choosing a Target

The picker lists agents and plain panes separately, because a message and a
keystroke want different kinds of target:

```
agents
  1  ○ idle     codex    wX:p1    ~/projects/website         website
  2  ● working  claude   wS:p4    ~/projects/shop-backend    running tests

panes
  3  · zsh      wN:p3    ~/projects/analytics
  4  · btop     wX:p2    ~/projects/website

target [1]:
```

`○` idle · `✓` done · `●` working · `◐` blocked · `?` unknown · `◷` limited ·
`·` plain pane. Colour is dropped when the output is not a terminal, and under
`NO_COLOR`. Enter accepts the highlighted default, which is the best-ranked
target: idle and done agents first, then unknown, working, blocked, then plain
panes.

An agent held by its own usage limit is drawn as `◷ limited`, with its reset
after the title. Another agent of the same kind that may share its account
(see [Resuming After a Usage Limit](#resuming-after-a-usage-limit)) is marked
`account limited`:

```
  1  ◷ limited  claude   wB:p1    ~/projects/shop-backend    checkout-webhook-migration  resets 16:00 (weekly)
  2  ○ idle     claude   wA:p1    ~/projects/analytics       Analytics backend rewrite  account limited
```

Without `--at` the send waits for a limit to lift, so limited agents come first,
the soonest reset (or a limit already over) at the top, and `--pick best` takes
that one. With `--at` the order is unchanged and the marks are only shown.

The pane you are running in is listed but never auto-selected — targeting it
needs `--allow-self`. Without that guard an agent could schedule a prompt to
itself.

Selectors:

| Flag | Effect |
| --- | --- |
| `--pane ID` | Target a pane or agent pane directly. |
| `--cwd PATH` | Match on working directory. Prefers agents; falls back to plain panes only when no agent matches. |
| `--kind NAME` | Only agents of that kind, e.g. `claude`, `codex`. |
| `--panes-only` | Only plain panes, never agents. |
| `--pick unique\|best` | Non-interactive tie-break. `unique` (default) fails on ambiguity; `best` takes the top rank. |
| `--allow-self` | Permit targeting the calling pane. |

Non-interactively, an ambiguous match prints the same grouped listing to stderr
and exits 1, so you can see what to narrow. `--pick best` resolves it instead
and names what it took, in the same form as the single-match note.

Every selection that skips the picker says which agent it landed on, so an
unattended run leaves a record of the choice rather than only the pane id.

## Sending

The payload decides the surface, and both are atomic:

| Target | Command used |
| --- | --- |
| agent, message | `herdr agent prompt`, then the submit gate below |
| plain pane, message | `herdr pane run` |
| either, `--keys` | `herdr agent send-keys` / `herdr pane send-keys` |

Because `agent prompt` honours the pane's bracketed-paste mode, a multi-line
`--message-file` arrives as **one** prompt. Sending the same text as raw
keystrokes would submit it line by line as several truncated turns.

### Confirming the submit

`agent prompt` sends the text and then an Enter, and Herdr reports success once
it observes the agent change lifecycle state. That observation is not a
receipt. Codex has no state hook: Herdr scrapes its state from the terminal
title and its idle rule matches any non-empty title, so Codex's state flaps on
its own, and a prompt whose Enter the TUI dropped still satisfied `--wait`.
Herdr records the mirror-image bug for Copilot, where a background pane
swallowed the keystrokes entirely until prompting was made to focus the pane
first. Either way the text is left in the composer as an unsubmitted draft
while the job reports success and cleans itself up.

So the send is gated on evidence instead, and `--wait` is not used at all. When
it sees no state change it fails with `agent_prompt_stalled` — the very case
worth recovering from — and when it sees one it holds the job open for the rest
of the turn, so the check would land minutes after the moment it could still
fix anything.

Two questions are asked after the text goes out, and the composer outranks the
lifecycle.

**Is the draft still on screen?** The message's last 24 characters are looked
for in the composer, with both sides reduced to lower-case alphanumerics so
that wrapping, padding, and gutter glyphs cannot break the match. Separately,
if the composer was empty before the send, anything in it afterwards is a stuck
draft — which is what catches a long paste that Claude collapses to
`[Pasted text #1 +40 lines]`, where the needle is not on screen to be found.
The composer is read only as far as the rule that closes it, because below that
rule sits a status footer whose context, cost, and elapsed-time counters move
on their own.

**Did the agent pick a turn up?** `agent get` reports `state_change_seq`, a
counter stamped on an agent whenever its own status transitions. Comparing it
against a baseline taken before the send is edge-triggered, so it still catches
a turn that started and finished between two polls — which comparing `working`
against `idle` would miss.

A draft that is still visible beats any state the agent claims, so a flapping
agent can no longer talk its way to a receipt. When the grace window closes
with the draft still there, the ladder runs:

1. press Enter again;
2. focus the pane, press Enter again, then hand focus back — `--recover focus`;
3. fail loudly, quoting the draft left in the composer.

Only evidence of a stuck draft authorises a keystroke, and the ladder presses
Enter without ever re-sending the text: if the first Enter did land after all,
a second copy of the message is far worse than an Enter into an empty composer,
which every supported TUI ignores. The ladder is abandoned if the agent has
meanwhile opened a dialog, where Enter would answer the dialog instead.

When neither question can be answered — nothing in the composer and nothing
moved — the send is logged as `(unverified)` rather than guessed either way.
Pressing Enter into a composer with no visible draft would be the blind
keystroke this gate replaced.

| Flag | Effect |
| --- | --- |
| `--recover keys` | Default. One extra Enter when the draft is stuck. |
| `--recover focus` | Then focus the pane and try once more, restoring focus after. |
| `--recover none` | Never press anything; fail if the submit is unconfirmed. |
| `--submit-grace SECS` | How long each attempt waits for the submit. Default 4. |

A recovered send is logged as `(Enter retried)` or `(focused, Enter retried)`,
and a failed one takes the journal, the notification, and a non-zero exit with
it rather than reporting a success that never happened.

### Still usage-limited

An agent that is out of usage accepts a prompt like any other — the composer
clears and the lifecycle moves — and then answers with a limit notice instead
of a turn. So once the submit is settled, the job watches the transcript for
10 seconds. Both Codex and Claude echo a submitted prompt, so the check looks
only below the last copy of the message's end, where a notice from an earlier
limit cannot be. A notice there — Codex's `■ You’ve hit your usage limit` or
`Usage limit reached`, Claude's `You've hit your … limit` — fails the job and
quotes the notice. The job does not re-arm itself; running the command again
without `--at` schedules from the new notice.

Claude draws on the alternate screen, which never reaches the host scrollback,
and Herdr can recover that history only from an idle agent. When a recent read
comes back empty, the visible screen stands in for it; it still holds the
latest exchange.

### Sending keys

```zsh
schedule-send --at '09:00' --pane wS:p1 --keys 'ctrl+c'
schedule-send --at '09:00' --cwd ~/projects/website --keys 'esc'
```

`--enter-only` is kept as an alias for `--keys enter`.

Keys go in as keys, with no submit gate: `esc` and `ctrl+c` change nothing that
could be verified the way a submitted prompt can.

For agent targets, `--if-busy` decides what happens if the agent is mid-turn at
fire time: `wait` (default) waits up to 15 minutes for it to settle, `abort`
cancels the send. A wait that runs out fails the send rather than typing into
a turn that is still running.

## Fire-Time Validation

The target is checked again the moment the timer fires. The send is refused if
the pane is gone, is no longer the same kind of thing, now runs a different
agent, or has moved to a different directory. A refusal raises a Herdr
notification and is recorded in the journal; nothing is typed.

This is the case that matters at 03:30: if the agent exited hours ago, the pane
is a bare shell, and blind text would be executed as a shell command.

`--force` skips the check. `--quiet` suppresses the failure notification.

## Managing Scheduled Sends

```zsh
schedule-send --list
schedule-send --status <id>
schedule-send --cancel <id>
```

On a terminal `--list` is one row per job, with how soon it fires. A timer that
is not active is marked, and its state stands in for how soon:

```
  ○ Sat 16:01   in 39m    claude wB:p1   "resume"                   20261003-160100-wBp1
  ✗ Sun 09:00   inactive  pane wB:p5     keys: ctrl+c               20261004-090000-wBp5
```

Piped, it stays a table of id, time, target, timer state and the payload's
first line. `--status` prints the stored job, the timer and the job's journal.
`--cancel` disarms the timer and removes its unit and state files.

Underneath, each job is a `herdr-send-<id>.timer` plus `.service` in
`~/.config/systemd/user`, so the plain systemd tools work too:

```zsh
systemctl --user list-timers 'herdr-send-*'
journalctl --user -u 'herdr-send-<id>.service'
```

Jobs clean up their own unit files after firing. Stale ones left by a hard kill
are swept the next time you schedule something.

## Missed Runs

If the machine is down at the target time the job is skipped by default, on the
grounds that a "resume work" prompt delivered six hours late is worse than none.
Pass `--persistent` to have it fire at the next boot instead.

## Dry Run

```zsh
schedule-send --at '03:30' --cwd ~/projects/website --message hello --dry-run
```

Resolves the target and prints what would be armed, without creating anything.
On a terminal:

```
  · dry run: would schedule "hello" for codex wV:p1
    at       Sun 03:30   in 11h
    cwd      ~/projects/website
    if-busy  wait
    recover  keys
    grace    4s
    persist  no
    id       20261004-033000-wVp1
    units    ~/.config/systemd/user/herdr-send-20261004-033000-wVp1.{service,timer}
    runs     schedule-send __fire 20261004-033000-wVp1
```

Without `--at` it still reads the notice and asks for confirmation, so a dry
run shows exactly what would be armed. Piped, the same facts come out as
`key=value` lines.

## Safety Test

Confirm a target is the one you meant before committing to a 03:30 send:

```zsh
herdr pane send-keys 'wX:p1' enter
```

## Notes

- Explicit full timestamps such as `2026-06-24 15:50:00` never roll forward. If
  they are in the past schedule-send exits instead of sending immediately.
- A job id is the fire time plus the pane, so a second send to the same pane at
  the same time is refused until the first is cancelled. Running the command
  twice on the same usage-limit notice lands on that case.
- The herdr binary path is resolved once at schedule time and stored with the
  job, so the worker does not depend on a login shell or `PATH`.
- Message text is stored verbatim in `~/.local/state/herdr-send/<id>.msg`; that
  directory is created mode 700.
- `HERDR_SCHEDULE_NOW_EPOCH` overrides "now" for testing `--dry-run`, including
  deciding whether a usage-limit notice has already passed.

## Find Targets

```zsh
herdr agent list
herdr pane list
```
