# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-11-03

### Added

- The picker marks a target that already has a send scheduled, with when it
  fires, and lists it after the targets still waiting, so Enter and
  `--pick best` take one of those. Once a target is chosen, the run says it
  already has a send.

### Changed

- A time typed without seconds, such as `+30`, fires on a whole minute and
  is shown without seconds; seconds typed on purpose are kept. "One minute
  from now" after a limit has passed moves up to a whole minute too.
- The Herdr plugin needs Herdr 0.9.5 or newer, the release its checks run
  against.
- Releases are built with the newest stable Go, and building from source
  needs Go 1.27.3 or newer.
- Releases are built by GitHub Actions pinned to commits, not to tags that
  can be moved.

### Fixed

- Ctrl-Space on an empty answer no longer ends the run without scheduling
  anything; it does nothing, as it already did once text was typed.
- A time at "When?" that clashes with a send already scheduled to the target
  is caught there, and the question is asked again; the run used to end
  after both answers, losing the message typed for it.
- The picker and the prompt's top line fit a narrow terminal instead of
  wrapping.

## [0.1.0] - 2026-10-24

The first public release.

### Added

- Schedule a message, the contents of a file, or key presses into a running
  [Herdr](https://herdr.dev) agent or terminal pane, through systemd user
  timers that outlive the shell that created them.
- Resume after a usage limit: read the reset time from a Codex or Claude
  notice and propose sending `resume` one minute later.
- Choose the target with a picker, or by pane, working directory, or agent
  kind, with agents held by a limit ranked first.
- On a terminal, ask as guided steps: which target, what to send (Enter
  keeps `resume`; a shell pane has to be told), and when. Answers can be
  edited with the arrow keys, and Ctrl-C cancels before anything is written.
- `+30`, `+30m` and `+2h` as short forms of `+30 min` and `+2 hours`.
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
  scheduling questions for the focused agent or shell pane in an overlay. It
  needs Herdr 0.9.3.
