# Using schedule-send

The full reference. The [README](../README.md) has the install commands and a
quick start. `schedule-send --help` lists every option, and `--version` prints
the version.

- [Requirements and the installed path](#requirements-and-the-installed-path)
- [Resume after a usage limit](#resume-after-a-usage-limit)
- [Target selection](#target-selection)
- [Times and payloads](#times-and-payloads)
- [Delivery and failures](#delivery-and-failures)
- [Manage jobs and missed runs](#manage-jobs-and-missed-runs)
- [Output for scripts](#output-for-scripts)
- [The Herdr plugin](#the-herdr-plugin)
- [Storage](#storage)

## Requirements and the installed path

On Linux, `schedule-send` needs [Herdr](https://herdr.dev), GNU `date`,
`/usr/bin/env` from GNU coreutils, and `systemctl --user`. On macOS, it needs
Herdr, `launchctl`, and `gdate` from Homebrew's `coreutils`, which the formula
installs.

Each install records the path it arms jobs from, that is, creates their
timers from:

- the Homebrew formula: Homebrew's `opt` path;
- `install.sh`: `~/.local/bin/schedule-send`;
- `tools/install`: the checkout's binary.

Only a run from that path, or through a symlink to it, can arm a job. A
`go build` or `go run` binary, or one moved or copied elsewhere, cannot. Dry
runs, `--list`, `--status`, `--cancel`, and `--targets` still work in such a
build, and so do jobs fired by timers from an older version.

A dry run shows the command its timer would run: `runs` on a terminal, and
`would_run` in `key=value` and JSON output. A build that cannot arm warns and
leaves it out.

On Linux, each job's service runs `/usr/bin/env --` with the quoted executable
path, escaping `%` and `$` as `%%` and `$$`. That escaping stands in for
systemd's `:` prefix, which needs systemd 249 or newer. The installed path
must be a clean, absolute UTF-8 path without control characters or `=`, since
`env` reads `=` as a variable assignment. `tools/build` and arming apply the
same check. The recorded path itself must be the real file, not a symlink,
even though you can run it through one.

A scheduled job uses the newly installed binary when it fires, so upgrading
needs no rescheduling.

On Linux, jobs fire only while you are logged in, unless user lingering is on.
Check it, and turn it on if it shows `Linger=no`:

```sh
loginctl show-user "$USER" -p Linger
loginctl enable-linger
```

## Resume after a usage limit

Leave out `--at`, and `schedule-send` reads the selected agent's usage-limit
notice. It proposes `resume` for one minute after the reported reset:

```sh
schedule-send --cwd ~/projects/my-project
```

On a terminal, it asks:

1. **What to send.** Enter keeps `resume`, or type another message.
   `--message`, `--message-file`, `--keys`, or `--enter-only` skips this
   question.
2. **When.** Enter confirms the proposal. With no usage limit on the agent's
   screen, there is none; type a time. A typed time, such as `00:45`, `3pm`,
   `Sep 28 1am`, or `+30`, is shown back as the new proposal. Press Enter
   again to confirm it.

Edit an answer before Enter with Left, Right, Home, End, Backspace, Delete,
and Ctrl-U. Ctrl-C cancels, and nothing is written before the last answer.

For unattended runs, take the detected proposal with `--auto`:

```sh
schedule-send --cwd ~/projects/my-project --auto
schedule-send --cwd ~/projects/my-project --auto --dry-run
```

Without a terminal, leaving out `--at` requires `--auto`. `--auto` fails when
there is no usable proposal. On a shell pane, one with no agent, `--auto`
needs a message or keys, since `resume` would run as a shell command. On a
terminal, the question for a shell pane has no default.

Supported notices include Codex's `try again at Sep 27th, 2026 12:27 AM` and
Claude's `resets 3pm` or `resets Sep 28, 3:30pm`. A zone such as
`(Europe/Warsaw)` may follow.

The detector combines the newest visible notice with the session and weekly
gauges in the agent's footer. A limit that has already passed proposes one
minute from now. When the target shows no notice, another agent of the same
kind can supply an account-wide reset. If the target's own gauge shows usage
left in that window, it is on another account, and that reset is ignored.

Claude's time-only notices omit the day. If the footer cannot confirm the day
of a reset more than 12 hours away, the proposal is marked unconfirmed, and
`--auto` still takes it. A notice left on screen after its limit lifted can
make that proposal a day late. Check the reset `schedule-send` shows before
you confirm, or use `--at '+1 min'`. A notice without a reset time needs a
time given by hand.

## Target selection

A single matching target is selected, with a note on stderr. With several, a
terminal shows a picker first: type a row's number, or press Enter for the top
row. Ctrl-C or Ctrl-D cancels. Without a terminal, several matches fail
unless `--pick best` is given. `--targets` lists every pane, its ID, and each
agent's usage limit.

| Option | Effect |
| --- | --- |
| `--pane ID` | Select a pane directly, for example `wS:p1`. |
| `--cwd PATH` | Select targets in that directory or below it. Shell panes count only when no agent matches. |
| `--kind NAME` | Select agents of a kind, such as `claude` or `codex`. |
| `--panes-only` | Select shell panes, never agents. |
| `--pick unique\|best` | Without a terminal: fail on several matches (the default), or take the top-ranked one. |
| `--allow-self` | Include the pane you run it from, which is left out by default. |

Targets rank idle or done agents first, then unknown, working, and blocked
ones, then shell panes. Without `--at`, agents at their own usage limit lead:
expired limits, then the soonest known reset, then unknown ones.

## Times and payloads

`--at` takes a local time (`HH:MM`, `HH:MM:SS`, `3pm`, `12:27 AM`) or a GNU
`date` expression such as `+30 min`. A full timestamp is `2026-10-15 18:30:00`
or `2026-10-15T18:30:00`. `+30` and `+30m` are minutes, and `+2h` is hours. A
clock time already past today means that time tomorrow, even across a
daylight-saving change. A past timestamp is refused.

On Linux, timers store the instant in UTC. The receipt, the summary printed
when a job is scheduled, shows local time. `HERDR_SCHEDULE_NOW_EPOCH` fixes
the current time for testing dry runs.

Choose one payload:

| Option | Effect |
| --- | --- |
| `--message TEXT` | Text for an agent, or a command for a shell pane. |
| `--message-file PATH` | A file's contents, submitted as one prompt; several lines are safe. |
| `--keys KEYS` | Send space-separated keys, such as `ctrl+c` or `esc`. |
| `--enter-only` | Short for `--keys enter`. |

```sh
schedule-send --pane wS:p1 --at '09:00' --keys 'ctrl+c'
```

## Delivery and failures

At the job's time, `schedule-send` checks that the pane still exists and has
the same working directory. It must still hold the same agent, or still be a
shell pane. A message to an agent then waits while the agent is busy (see
`--if-busy`), and the check repeats. Keys, and messages to a shell pane, are
sent at once, unconfirmed.

| Option | Effect |
| --- | --- |
| `--if-busy wait\|abort` | Wait up to 15 minutes for a busy agent (the default), or fail unless it is idle or done. A wait that runs out fails the job. |
| `--recover keys` | The default: press Enter once more on fresh evidence that the text is still in the input box. |
| `--recover focus` | Like `keys`; if the text is still there, also focus the pane, press Enter once more, and hand focus back when it can. |
| `--recover none` | Never press Enter again. |
| `--submit-grace SECS` | Seconds each confirmation waits, 1 to 60; default 4. |
| `--force` | Skip the target checks. |
| `--quiet` | Raise no Herdr notification on failure. |

The text is sent once. To confirm it went in, `schedule-send` reads the input
box and the agent's state. Text still in the box outweighs a state change.
Recovery presses Enter, never the text again, and stops when:

- a dialog opens;
- the agent's state cannot be read just before Enter;
- there is no fresh evidence of the text in the box.

A failed screen read is not proof the text went in. So if the text was seen
in the box and a later read fails, the job fails. With no evidence either
way, the job is logged as unverified. Text known to be left in the box fails
it.

For 10 seconds after the text goes in, a new usage-limit reply fails the job.
It is not rescheduled; run `schedule-send` again without `--at` to use the new
notice. A Herdr error reply is a failure even with exit status 0, and every
external command has a deadline.

Each failure goes to the job's log and, unless `--quiet` is set, to a Herdr
notification. A failed scheduling attempt is rolled back; if that cannot be
done safely, the state stays for inspection.

## Manage jobs and missed runs

Each job has an ID, shown in its receipt and in `--list`:

```sh
schedule-send --list
schedule-send --status <id>
schedule-send --cancel <id>
```

`--list` shows each job's time, target, timer state, and message or keys, and
works without Herdr. `--status` shows the stored state, the timer's status,
and recent log entries. `--cancel` stops the timer, and the job itself if it
is running, such as one waiting for a busy agent. It then removes the job's
files. It reports anything it cannot stop or remove, and never claims success
then. A job that has run removes its own timer and state.

`--status` allows 30 seconds in all for the timer queries, then 10 seconds for
the log, and one second to drain each command's output. On Linux, an
inactive, failed, or missing unit is a normal result once its properties
confirm it, even with its file removed. Error output from such a query, such
as systemd's "changed on disk" note, is only a warning. A missing state file
is normal for a job that has run.

When a source cannot be read, `--status` exits 1, names the source, and keeps
the partial output. Causes include missing commands, deadlines, transport
errors, and unreadable or invalid state.

A job missed while the machine is down is skipped. Add `--persistent` when
scheduling to run it when your jobs next start instead: at boot on Linux with
lingering, otherwise at your next login.

On Linux, systemd's tools can inspect jobs directly:

```sh
systemctl --user list-timers 'herdr-send-*'
journalctl --user -u 'herdr-send-<id>.service'
```

On macOS, a job is a launchd agent, defined by a plist, in your login session.
It logs beside its state, and `--status` shows the log. An ordinary job's
plist also sits beside its state, so macOS shows no background-item notice,
and launchd forgets it at logout. A `--persistent` job's plist goes in
`~/Library/LaunchAgents`, which survives a restart and which macOS announces.
launchd starts a job a few seconds into its minute; the job then waits for its
exact second. A late job on macOS is:

- sent on wake, if the Mac was asleep;
- skipped if the Mac was off or you were logged out, unless `--persistent`;
- never sent once it is more than 7 days late.

On macOS, `launchctl` can inspect a job directly:

```sh
launchctl print gui/$(id -u)/herdr-send-<id>
```

## Output for scripts

On a terminal, output is styled; `NO_COLOR` turns colour off. When output is
redirected, a receipt is printed as `key=value` lines and a list as a plain
table. Add `--json` for structured output:

```sh
schedule-send --targets --json
schedule-send --list --json
schedule-send --status <id> --json
schedule-send --pane wS:p1 --at '16:30' --message hello --dry-run --json
```

With `--json`, stdout holds one result object, and stderr holds separate note,
warning, and error objects. An error looks like
`{"error":{"code":"...","message":"..."}}`, with the code `usage`,
`not_found`, `ambiguous`, `cancelled`, or `failed`. `--json` still asks its
questions, so a script should name the target and give `--at` or `--auto`.

Ctrl-C at a question exits 130, an unknown option exits 2, and other failures
exit 1.

Status JSON has `state`, `timer`, `journal`, and optional `warning` and
`diagnostics`. A diagnostic has `source`, `severity`, `code`, and `message`;
one from a command adds `exit_code`, `stderr`, and partial `output` when it
has them. On exit 1, stdout still holds one partial result, and stderr the
error. Command warnings alone never cause exit 1. A job's own log stays plain
text.

## The Herdr plugin

The plugin, `dj-skn.schedule-send`, has one action, `schedule`. It opens an
overlay over the focused pane and runs `schedule-send --pane <that pane>` in
it. Bind the action to any free key in Herdr's `config.toml`, for example:

```toml
[[keys.command]]
key = "prefix+alt+s"
type = "plugin_action"
command = "dj-skn.schedule-send.schedule"
description = "schedule a send"
```

The overlay asks what to send and when. On an agent, Enter keeps `resume`;
a shell pane needs an answer. On an agent held by a usage limit, Enter takes
one minute after the reset; otherwise, type a time. Ctrl-C cancels. The
overlay stays open until you press Enter, so you can read the receipt.

Options for every job scheduled from the plugin go in a file named `args`, in
the plugin's configuration directory. This prints that directory:

```sh
herdr plugin config-dir dj-skn.schedule-send
```

Put one or more options on each line, separated by spaces; quotes are not
read. A line that starts with `#` is a comment:

```text
# Preview instead of scheduling
--dry-run
```

The plugin looks for the program on `PATH`, then in `~/.local/bin` and
Homebrew's directories, because Herdr starts plugin commands with its
server's `PATH`. Its install step leaves an existing install alone. When the
program is missing, it runs `install.sh` on Linux, and on macOS it stops and
prints the Homebrew command.

## Storage

Job state and messages live in `~/.local/state/herdr-send`, mode 700, with new
files mode 600. On Linux, timer units live in `~/.config/systemd/user`. On
macOS, the plists live beside the state, or in `~/Library/LaunchAgents` for
`--persistent` jobs. `XDG_STATE_HOME`, and on Linux `XDG_CONFIG_HOME`, move
those locations.

A job's state records the target's identity, the resolved Herdr executable,
and, for new jobs, the firing time as a Unix epoch (`run_at`). Older job files
still work: a missing or null number takes its old default, `force=0`,
`quiet=0`, `grace=4`, or `run_at=0`. A given number must be a JSON integer:
`force` and `quiet` 0 or 1, `grace` 1 to 60, and `run_at` a non-negative
64-bit integer.

Invalid state stops delivery and keeps the job for inspection, with a Herdr
notification unless the job loaded as quiet. `--list` warns about state it
cannot load, or whose `id` differs from its file name. It shows such a job as
`invalid`, with an `error` field in JSON. `--status` shows the stored
document.

A pane and a time identify a job, so the same pair cannot be scheduled twice.
Scheduling, cancelling, and cleanup share a lock for each job. Its hidden lock
file stays on purpose, so competing processes cannot take different locks for
one job.
