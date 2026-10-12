# Herdr Scheduled Send

`schedule-send` schedules a message or key presses into a running Herdr agent
or terminal pane. It can also read a Codex or Claude usage-limit notice and
schedule `resume` just after the limit resets.

Jobs use systemd user timers, so the launching shell does not need to stay
open. The target is checked again before delivery. Linux only.

## Install and automatic updates

Build requirements: Go **1.27.1 or newer**, Git, `flock`, and GNU coreutils
(`base64` and `tr`). Running the app requires Herdr, GNU `date`, `/usr/bin/env` (GNU coreutils), and
`systemctl --user`.

From the main checkout on `main`, run once:

```zsh
cd ~/src/schedule-send
tools/setup-hooks
hash -r
schedule-send --help
```

`tools/setup-hooks` enables this repository's tracked Git hooks and installs
when needed. After a commit, merge/pull, checkout, or history rewrite, the
hooks rebuild changed program or installer inputs on a **clean main checkout
on `main`**. Documentation-only updates do not rebuild. Feature branches,
linked worktrees, detached HEADs, and dirty checkouts are skipped. A main
checkout still on `master` is skipped with a hint to run
`git branch -m master main`.

The hook compares committed input hashes with the installed build. Changes to
`tools/install` itself trigger a rebuild. A failed build reports a retry
command and keeps the previous binary. Hooks do not run on ordinary file saves;
a skipped dirty checkout needs a later clean Git update or a manual install:

```zsh
tools/install
```

Installation builds `bin/schedule-send`, replaces it atomically, and links
`~/.local/bin/schedule-send` to it. Only one installation can run at a time.
Installer builds record the stable main-checkout binary path. Real scheduling
requires running from that location (including through its symlink). Plain
`go build`/`go run` builds and moved or copied binaries cannot arm jobs; run
`tools/install` from the main checkout on `main` and invoke the installed command. Dry-run,
management commands, and legacy timer firing remain available in plain builds.
Generated services invoke `/usr/bin/env --` with the quoted executable path.
Percent specifiers and dollar signs are escaped (`%%`, `$$`), which works on
systemd releases older than the `:` prefix. The installed path must be clean,
absolute UTF-8 without control characters or `=` (env would read `=` as a
variable assignment); `tools/build` and arming apply the same check, and the
recorded path must be the installed file itself, not a symlink.
Already-scheduled jobs use the newly installed binary when they fire. The zsh
shim remains so timers created before the Go rewrite still work.

Hook configuration is local to each repository; run `tools/setup-hooks` after
a fresh clone. Setup refuses to overwrite a different `core.hooksPath`.
To disable these hooks:

```zsh
git config --local --unset core.hooksPath
```

For timer survival across logout/reboot, check that user lingering is enabled:

```zsh
loginctl show-user "$USER" -p Linger
```

## Quick start

List running targets:

```zsh
schedule-send --targets
```

Schedule a message, choosing the target from a picker:

```zsh
schedule-send --at '15:50' --message 'resume, then commit when done'
```

Select a project and send a file as one prompt:

```zsh
schedule-send --cwd ~/projects/my-project --at '03:30' --message-file prompt.txt
```

Preview the target and schedule without creating a job:

```zsh
schedule-send --cwd ~/projects/my-project --at '03:30' --message hello --dry-run
```

Receipts include the job ID and a cancellation command.

## Resume after a usage limit

Omit `--at` to read the selected agent's limit notice. The default message is
`resume`, and the proposed time is one minute after the reported reset:

```zsh
schedule-send --cwd ~/projects/my-project
```

Enter confirms the proposal. A replacement such as `00:45`, `3pm`,
`Sep 28 1am`, or `+30 min` is shown back and becomes the new default;
press Enter again to confirm it. `--message` overrides `resume`.

For unattended use, accept the detected proposal with `--auto`:

```zsh
schedule-send --cwd ~/projects/my-project --auto
schedule-send --cwd ~/projects/my-project --auto --dry-run
```

Without a terminal, omitting `--at` requires `--auto`. Automatic scheduling
fails when there is no usable proposal. A plain pane needs an explicit time
and payload because `resume` would otherwise run as a shell command.

Supported notices include Codex's `try again at Sep 27th, 2026 12:27 AM` and
Claude's `resets 3pm` or `resets Sep 28, 3:30pm`, optionally followed by a
zone such as `(Europe/Warsaw)`.

The detector combines the newest visible notice with session/weekly footer
information. A limit already over proposes one minute from now. Another agent
of the same kind can supply an account-limit reset when the target has no
notice; a target's own available-usage gauge can rule that peer out.

Claude's time-only notices omit the day. If the footer cannot disambiguate a
reset more than 12 hours away, it is marked unconfirmed; `--auto` still takes
that proposal. Inspect the report or use `--at '+1 min'` when a notice is stale.
A notice without a reset time requires a manually supplied time.

## Target selection

With one matching target, selection is automatic and reported on stderr.
Otherwise a terminal gets a picker; Enter accepts its default and EOF cancels.
Without a terminal, an ambiguous match fails unless `--pick best` is given.

| Option | Effect |
| --- | --- |
| `--pane ID` | Select a pane directly, for example `wS:p1`. |
| `--cwd PATH` | Match the directory and descendants; prefer matching agents over plain panes. |
| `--kind NAME` | Select agents of a kind, such as `claude` or `codex`. |
| `--panes-only` | Select plain panes. |
| `--pick unique\|best` | Fail on ambiguity by default, or take the highest-ranked target. |
| `--allow-self` | Permit targeting the calling pane. |

Normal ranking favors idle/done agents, then unknown, working, blocked, and
plain panes. Without `--at`, agents held by their own limits come first;
expired limits and sooner known resets rank ahead of later/unknown resets.
The calling pane is excluded unless `--allow-self` is set.

## Times and payloads

`--at` accepts local `HH:MM[:SS]`, `H[:MM]am/pm`, full timestamps, and GNU date
expressions such as `+30 min`. Clock-only values already passed today roll to
tomorrow using calendar days. Explicit past timestamps are rejected. Timers
store the resolved instant in UTC; receipts show local time.

Choose one payload:

| Option | Effect |
| --- | --- |
| `--message TEXT` | Send text to an agent or run it in a plain pane. |
| `--message-file PATH` | Read a file and submit its contents as one prompt. |
| `--keys KEYS` | Send space-separated keys, such as `ctrl+c` or `esc`. |
| `--enter-only` | Alias for `--keys enter`. |

```zsh
schedule-send --pane wS:p1 --at '09:00' --keys 'ctrl+c'
```

## Delivery and failures

Before sending, the worker checks that the pane still exists, has the same
kind/agent, and has the same working directory. For **agent messages**, the
busy policy applies, followed by another target check. Keys are delivered
without waiting for an agent turn or verifying prompt submission.

| Option | Effect |
| --- | --- |
| `--if-busy wait\|abort` | For agent messages: wait up to 15 minutes by default, or require idle/done immediately. |
| `--recover keys` | Default: retry Enter once when fresh evidence shows a stuck draft. |
| `--recover focus` | Also focus the pane and retry Enter, then restore the previous agent focus when available. |
| `--recover none` | Do not retry Enter. |
| `--submit-grace SECS` | Confirmation window per attempt, 1–60 seconds; default 4. |
| `--force` | Skip target identity/directory checks. |
| `--quiet` | Suppress failure notifications. |

Agent text is sent once. The submission check reads the composer and agent
lifecycle evidence; a visible draft outweighs a reported state change. Recovery
presses Enter without resending the message, and stops if a dialog opens,
the agent's state cannot be read just before Enter, or fresh draft evidence is
unavailable. A read failure cannot establish that a
seen draft was submitted. A send with no usable submission evidence is logged
as unverified; a known unsent draft fails the job.

After submission, the worker watches for a new usage-limit reply for 10 seconds.
A detected rejection fails the job; it does not automatically reschedule it.
Run the resume command again to use the new notice. Herdr error replies are
checked even when the process exits 0, and external commands have deadlines.

Failures are recorded in the journal and normally raise a Herdr notification.
Scheduling rolls back failed attempts. If partial activation cannot be safely
undone, state is retained for inspection. Cancellation also reports stopping
or cleanup failures instead of claiming success.

## Manage jobs and missed runs

Use the ID from a receipt or `--list`:

```zsh
schedule-send --list
schedule-send --status <id>
schedule-send --cancel <id>
```

`--list` shows time, target, timer state, and payload; it works even if Herdr
is missing. `--status` includes stored state, timer status, and recent journal
entries. Cancellation stops the timer/service and removes the job's files.
Completed jobs clean up their own units and state.

Status inspection gives timer status and any confirming unit-state query a
shared 30-second budget, then gives the journal 10 seconds. Command pipe drains
are limited to one second. Inactive, failed, and missing units are normal
inspection results when their unit properties confirm the condition, including
a unit whose file was removed; stderr from such a status read, like systemd's
"changed on disk" note, is a warning. A missing state file is normal for a
completed job. Collection failures (including missing
commands, deadlines, transport errors, unreadable state, or invalid state JSON)
exit 1, name the failing source, and retain partial output.

Jobs missed while the machine is down are skipped by default. Add
`--persistent` when scheduling to run a missed job at the next boot.

Systemd tools can inspect jobs directly:

```zsh
systemctl --user list-timers 'herdr-send-*'
journalctl --user -u 'herdr-send-<id>.service'
```

## Output for scripts

Terminal output is styled; `NO_COLOR` disables color. Redirected scheduling
receipts use `key=value`, and redirected lists use a plain table. Add `--json`
for structured output:

```zsh
schedule-send --targets --json
schedule-send --list --json
schedule-send --status <id> --json
schedule-send --pane wS:p1 --at '16:30' --message hello --dry-run --json
```

JSON mode emits one result object on stdout and separate note/warning/error
objects on stderr. Errors use `{"error":{"code":"...","message":"..."}}`,
with `usage`, `not_found`, `ambiguous`, or `failed` codes. JSON does not disable
interactive prompts; use explicit selectors and `--at` or `--auto` for scripts.
Status JSON retains `state`, `timer`, `journal`, and optional `warning`, and adds
optional `diagnostics` with `source`, `severity`, `code`, and `message`. Command
diagnostics may include `exit_code`, `stderr`, and partial `output`. Even when
inspection exits 1, stdout contains exactly one partial status result, with the
usual error envelope on stderr. Command warnings alone do not cause exit 1.
The internal worker's journal output remains plain text.

## Structure and storage

| Location | Responsibility |
| --- | --- |
| `cmd/schedule-send` | Program entrypoint. |
| `internal/cli` | Arguments, picker, scheduling, and management commands. |
| `internal/target`, `limit`, `notice`, `when` | Target matching, usage-limit detection, and time parsing. |
| `internal/herdr`, `screen` | Herdr calls and terminal-screen interpretation. |
| `internal/job` | State files, locking, timers, and cleanup. |
| `internal/installpath` | The recorded installation path and the path rules `tools/build` shares. |
| `internal/worker` | Delivery and submission verification. |
| `internal/show`, `term` | Text/JSON output and terminal handling. |
| `.githooks`, `tools` | Automatic installation, builds, and verification. |

State and messages live in `~/.local/state/herdr-send` (directory mode 700;
new files mode 600). Units live in `~/.config/systemd/user`. `XDG_STATE_HOME`
and `XDG_CONFIG_HOME` override those roots. State records the target identity,
resolved Herdr executable, and, for new jobs, the firing epoch (`run_at`).
Older job files remain supported. Missing or null numeric fields retain legacy
defaults: `force=0`, `quiet=0`, `grace=4`, and `run_at=0`. Supplied values must
be JSON integer literals: force/quiet are 0–1, grace is 1–60, and run_at is a
nonnegative 64-bit integer. Invalid numeric state stops delivery and retains
the job for inspection; `--list` warns and skips it, while `--status` shows
the stored document.

A pane/time pair identifies a job; duplicate scheduling is refused. Scheduling,
cancellation, and cleanup share a per-job lock. Hidden lock files deliberately
remain so competing processes cannot acquire different locks for one job.

## Development

Use a feature worktree and check changes before committing:

```zsh
tools/check                  # formatting, vet, Go tests, shim and hook checks
tools/parity                 # compare with the tagged zsh baseline
```

Checks use temporary files and stand-in Herdr/systemd commands, never real
panes or timers. Hook tests use temporary Git repositories and a mock installer.
Checks need `zsh` and `jq`; parity additionally needs Python 3 and `script`.
`zsh-baseline` is the stable Git tag used by parity, so keep it when copying
or fetching the repository. See [AGENTS.md](AGENTS.md) for contributor rules.

`HERDR_SCHEDULE_NOW_EPOCH` overrides the scheduling clock for tests, including
notice timing. Use it with stand-ins or `--dry-run`, not real scheduling.
