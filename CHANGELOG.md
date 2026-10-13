# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

The first public release.

### Added

- Schedule a message, the contents of a file, or key presses into a running
  [Herdr](https://herdr.dev) agent or terminal pane, through systemd user
  timers that outlive the shell that created them.
- Resume after a usage limit: read the reset time from a Codex or Claude
  notice and propose sending `resume` one minute later.
- Choose the target with a picker, or by pane, working directory, or agent
  kind, with agents held by a limit ranked first.
- Recheck the target before delivery, confirm that an agent took the prompt,
  and retry Enter only on fresh evidence of a stuck draft.
- List, inspect, and cancel jobs, with `--json` for scripts and `--dry-run`
  to preview a schedule without creating it.
- macOS support, through launchd agents in the login session. A job whose
  time passes while the Mac is asleep is sent on wake; `--persistent` also
  covers a restart.
- Install with Homebrew on macOS or Linux, or with `install.sh` on Linux,
  which downloads a checked release binary to `~/.local/bin`.
- `--version`.
- A Herdr plugin, `dj-skn.schedule-send`, with one action that opens the
  scheduling prompt for the focused agent in an overlay. It needs Herdr 0.9.3.
