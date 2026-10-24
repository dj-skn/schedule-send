# Repository guidance

## Project and entrypoints

`schedule-send` is a Go CLI for Linux and macOS that schedules messages or keys
into Herdr agents/panes through on-disk timers: systemd user timers on Linux,
launchd agents on macOS. [README.md](README.md) shows what it does and how
it is installed, [docs/usage.md](docs/usage.md) is the full reference, and
[CONTRIBUTING.md](CONTRIBUTING.md) has the package map and the checkout
install. `cmd/schedule-send` is the entrypoint; `schedule-send.zsh` is the
compatibility shim for older timers.

## Workflow

- Keep the main checkout on `main`. Develop and build feature changes in
  linked worktrees, kept in a `<checkout>.worktrees` directory beside it.
- Run `tools/check` before committing. For delivery or scheduling changes,
  run the relevant `tools/parity` cases or the full suite as appropriate.
- CI (`.github/workflows/ci.yml`) runs these checks on Linux, Linux ARM, and
  macOS; a `v*` tag runs `.github/workflows/release.yml`. Prepare releases with
  `tools/release`, never by editing versions by hand.
- Use the user's commit prefixes: `feat:`, `fix:`, `docs:`, `refactor:`,
  and `chore:`. Use `chore:` for merge commits and tooling changes.
- `tools/install` and `tools/setup-hooks` must run only from the main checkout
  on `main`; both enforce this through `tools/main-checkout`, the one place
  that names the deploy branch. Never point the installed binary at a worktree.
- Tracked Git hooks automatically install changed program/installer inputs on
  a clean main checkout on `main` after commits, merges/pulls, checkouts, and
  rewrites. They skip other branches, linked worktrees, dirty trees, and
  unchanged inputs; a main checkout left on `master` gets a rename hint.
  File saves do not run hooks. Explicit manual installation remains available.
- `tools/setup-hooks` enables hooks once per repository and installs if needed.
  Preserve existing custom hook configuration; setup refuses to replace it.

## Commands

| Command | Purpose |
| --- | --- |
| `tools/check` | Formatting, vet for Linux and macOS, Go tests, zsh shim syntax, shell syntax, and isolated hook tests. |
| `tools/parity [CASE...]` | Compare behavior against the zsh baseline in `testdata/baseline` with documented differences checked. |
| `tools/launchd-smoke` | Arm real launchd jobs aimed at the stand-in Herdr and check they fire and clean up; macOS only. |
| `tools/install` | Build, atomically publish, and link the binary; main checkout on `main` only. |
| `tools/dist VERSION` | Build the Linux release archives and their checksum file. |
| `tools/install-smoke` | Run `install.sh` against archives built from the checkout, under a temporary home; Linux only. |
| `tools/brew-smoke` | Install the formula from a temporary local tap and check it, then uninstall; needs Homebrew. |
| `tools/plugin-smoke` | Check the Herdr plugin's scripts against stand-ins, and its manifest with Herdr under temporary roots. |
| `tools/release VERSION` | Date the changelog's Unreleased entries, set the plugin's version, commit, and tag; pushes nothing. |
| `tools/bump-tap [--dry-run] VERSION` | Write the formula, with the tag's archive URL and checksum, to the Homebrew tap when it differs; run by the release workflow. |
| `tools/release-check VERSION` | Check that the changelog and the plugin manifest carry the version, and print its release notes. |
| `tools/setup-hooks` | Enable this repository's hooks and install if needed; main checkout on `main` only. |
| `tools/media [TAPE...]` | Record the README's GIFs from `docs/media/*.tape` with VHS in Docker, against `testdata/demo` and the stand-ins. |

Build/install needs Go 1.27.1+, Git, `flock`, and GNU coreutils (`base64`, `tr`). Checks also need `zsh` and
`jq`; parity uses Python 3 and `script`, and runs on Linux only. Runtime on Linux needs Herdr, GNU `date`,
`/usr/bin/env` (GNU coreutils), and `systemctl --user`; on macOS, Herdr, `launchctl`, and GNU `date` as `gdate`.

## Compatibility and delivery invariants

- Never test against real panes or arm real timers. Use the stand-ins under
  `testdata`, temporary directories, and mock installers for hook tests. The
  one exception is `tools/launchd-smoke`, whose real launchd jobs deliver only
  to the stand-in Herdr. A script that runs the real `herdr` first clears
  every `HERDR_` variable, which inside a Herdr pane leads to the live server.
- Each system's scheduler is one file in `internal/job` (`systemd_linux.go`,
  `launchd_darwin.go`) providing the same functions. Code outside those files
  names neither system. launchd kills a job that boots itself out, so a job's
  cleanup there removes its files and state first and unloads itself last.
  Only a `--persistent` job's agent goes in `~/Library/LaunchAgents`: macOS
  announces every file written there.
- Armed jobs run an installed binary, at the path compiled into it: the main
  checkout's, Homebrew's `opt` path, or `~/.local/bin` for a release. A
  recorded path is absolute or begins `~/`; arming from anywhere else is
  refused. Preserve the zsh shim and
  old state-file compatibility. Add state keys with defaults; never rename or
  remove existing keys. Validate required fields before delivering.
- Recheck target identity and working directory before sending, including
  after busy waits. Treat Herdr JSON error replies as failures even with exit 0.
- A screen-read failure is not proof of submission, and a failed lifecycle
  read is not proof that no dialog is open. Retry Enter only with fresh draft
  evidence; never resend text or answer a dialog as recovery.
- Preserve exclusive job reservation, atomic file publication, rollback, and
  honest cancellation errors. Keep hidden lock files to retain inode identity.
- Store new timer instants in explicit UTC while keeping human output local.
  launchd calendars are in the system zone, without seconds or a year, so the
  worker checks the stored instant before delivering.
  Keep JSON diagnostics structured and older optional state defaults intact.
- Keep `testdata/baseline/schedule-send.zsh`, the zsh script the program was
  ported from, unchanged for parity.

## Stories

Use story names such as `SS-1`, plans in `docs/ss-<n>-<slug>-plan.md`, and
branches `ss-<n>` with optional phase branches `ss-<n>-p<k>`. Plans record the
story's history; current behavior is defined by code, tests, the README, and
`docs/usage.md`.
