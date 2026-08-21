# Herdr Scheduled Send

Small helper for scheduling text and key input into a running Herdr pane. It works on this Linux VPS and still supports macOS.

## Install Once

The command is exposed through a stable symlink in `~/.local/bin`, which is
already on this account's `PATH`:

```zsh
ln -s "$HOME/src/schedule-send/schedule-send.zsh" "$HOME/.local/bin/schedule-send"
hash -r
```

Verify it from any directory:

```zsh
command -v schedule-send
schedule-send --help
```

## Command

```zsh
schedule-send
```

`schedule-send` is a symlink in `~/.local/bin` to this script. Because it
targets the source file directly, updates to
`~/src/schedule-send/schedule-send.zsh` are used immediately—there is nothing to
reinstall after editing or pulling updates. Recreate the link only if the
script is moved to a different directory.

## Basic Usage

Schedule a prompt to a Herdr pane:

```zsh
schedule-send \
  --at '15:50' \
  --message 'resume, then commit when done'
```

If there is only one Herdr pane, the script uses it automatically. If there are multiple panes, it shows a numbered list with pane id, agent, status, focus state, and cwd, then asks which one to target.

Pass `--pane` to skip selection:

```zsh
schedule-send \
  --pane 'wJ:p1' \
  --at '15:50' \
  --message 'resume, then commit when done'
```

Time-only values use local time. If the time already passed today, the script schedules it for tomorrow.

Example: if it is `23:40` and you pass `--at '03:30'`, it schedules for `03:30` tomorrow.

## Safety Test

Send Enter immediately to confirm the target pane is correct:

```zsh
herdr pane send-keys 'wJ:p1' enter
```

## Dry Run

Preview the schedule without starting anything:

```zsh
schedule-send \
  --at '03:30' \
  --message 'hello' \
  --dry-run
```

## Useful Options

`--enter-only`

Schedule only an Enter key send.

`--no-initial-enter`

Skip the initial Enter before sending the message.

`--message-file PATH`

Read the message from a file instead of passing it inline.

`--log PATH`

Choose a custom log path. By default logs go to `/tmp/herdr-scheduled-send-<pane>-<timestamp>.log`.

`--pane ID`

Choose the pane directly. This is required when running non-interactively and more than one pane exists.

## Check Running Scheduler

```zsh
pgrep -af 'setsid zsh|caffeinate.*zsh|herdr pane'
```

## Find Pane IDs

```zsh
herdr pane list
herdr agent list
```

## Notes

- The script uses `nohup` so it survives the launching shell closing.
- Pane auto-selection uses `herdr pane list` JSON and `jq`; pass `--pane` if `jq` is unavailable.
- On Linux, the script uses `setsid` when available so the scheduled job is detached from the launching shell. On macOS, it uses `caffeinate -i` when available so the Mac stays awake while display sleep is still allowed.
- The VPS uses GNU `date`; macOS uses BSD `date`. The script handles both.
- Explicit full timestamps such as `2026-06-24 15:50:00` do not roll forward. If they are in the past, the script exits instead of sending immediately.
