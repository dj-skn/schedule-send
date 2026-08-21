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

With no selector this shows a picker. With a selector that resolves to exactly
one target it runs silently — which is what makes it usable from scripts:

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
and exits 1, so you can see what to narrow.

## Sending

The payload decides the surface, and both are atomic:

| Target | Command used |
| --- | --- |
| agent, message | `herdr agent prompt --wait` |
| plain pane, message | `herdr pane run` |
| either, `--keys` | `herdr agent send-keys` / `herdr pane send-keys` |

Because `agent prompt` honours the pane's bracketed-paste mode, a multi-line
`--message-file` arrives as **one** prompt. Sending the same text as raw
keystrokes would submit it line by line as several truncated turns.

```zsh
schedule-send --at '09:00' --pane wS:p1 --keys 'ctrl+c'
schedule-send --at '09:00' --cwd ~/projects/website --keys 'esc'
```

`--enter-only` is kept as an alias for `--keys enter`.

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
