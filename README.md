# Herdr Scheduled Send

Schedule text or key presses into a running Herdr agent or pane. Linux only —
this is the VPS build and it depends on GNU `date` and `systemd --user`.

The delayed job is owned by an on-disk systemd user timer, so it survives the
launching shell, a logout, and a reboot. Targets are re-resolved at fire time,
so a pane that changed identity in the meantime is never written to.

## Install Once

```zsh
ln -s "$HOME/src/schedule-send/schedule-send.zsh" "$HOME/.local/bin/schedule-send"
hash -r
```

Verify:

```zsh
command -v schedule-send
schedule-send --help
```

The symlink points at the source file, so edits and pulls take effect
immediately. Recreate the link only if the script moves.

Requires `jq`, `systemctl --user`, and `herdr` on `PATH`. Lingering must be
enabled for reboot survival (`loginctl show-user "$USER" -p Linger`).

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

stdout stays the plain `key=value` receipt, so this is still usable from
scripts:

```zsh
schedule-send --at '03:30' --cwd ~/projects/website --message-file prompt.txt
```

Time-only values use local time. If the time already passed today it schedules
for tomorrow, re-parsed against tomorrow's calendar date so a DST transition
between the two days cannot shift the result by an hour.

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

`○` idle · `✓` done · `●` working · `◐` blocked · `?` unknown · `·` plain pane.
Colour is dropped when the output is not a terminal, and under `NO_COLOR`.
Enter accepts the highlighted default, which is the best-ranked target: idle and
done agents first, then unknown, working, blocked, then plain panes.

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

### Sending keys

```zsh
schedule-send --at '09:00' --pane wS:p1 --keys 'ctrl+c'
schedule-send --at '09:00' --cwd ~/projects/website --keys 'esc'
```

`--enter-only` is kept as an alias for `--keys enter`.

Keys go in as keys, with no submit gate: `esc` and `ctrl+c` change nothing that
could be verified the way a submitted prompt can.

For agent targets, `--if-busy` decides what happens if the agent is mid-turn at
fire time: `wait` (default) waits for it to settle, `abort` cancels the send.

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

`--list` shows id, time, target, timer state, and the payload's first line.
`--status` prints the stored job, the timer, and the job's journal. `--cancel`
disarms the timer and removes its unit and state files.

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

Resolves the target and prints the schedule, the unit name, and the unit file
paths without creating anything.

## Safety Test

Confirm a target is the one you meant before committing to a 03:30 send:

```zsh
herdr pane send-keys 'wX:p1' enter
```

## Notes

- Explicit full timestamps such as `2026-06-24 15:50:00` never roll forward. If
  they are in the past the script exits instead of sending immediately.
- The herdr binary path is resolved once at schedule time and stored with the
  job, so the worker does not depend on a login shell or `PATH`.
- Message text is stored verbatim in `~/.local/state/herdr-send/<id>.msg`; that
  directory is created mode 700.
- `HERDR_SCHEDULE_NOW_EPOCH` overrides "now" for testing `--dry-run`.

## Find Targets

```zsh
herdr agent list
herdr pane list
```
