# Contributing

Thanks for considering it. Bug reports and pull requests are both welcome.

## Getting set up

You need Linux with Go 1.27.1 or newer, Git, `flock`, GNU coreutils, `zsh`, and
`jq`. The parity suite also needs Python 3 and `script`.

```bash
git clone https://github.com/dj-skn/schedule-send.git
cd schedule-send
tools/check
```

Nothing has to be installed to work on it: the checks build what they need.

## The checks

Run both before you push:

```bash
tools/check                  # formatting, vet, Go tests, shell and hook checks
tools/parity                 # compare with the zsh baseline
```

`tools/parity` runs the same commands through the program and through the zsh
script it was ported from, and fails on any difference that is not documented
in the suite. Run it for anything that touches scheduling or delivery.

Checks use temporary files and stand-in Herdr and systemd commands, never real
panes or timers; hook tests use temporary Git repositories and a mock
installer. `HERDR_SCHEDULE_NOW_EPOCH` overrides the scheduling clock for tests,
including notice timing. Use it with the stand-ins or `--dry-run`, not for real
scheduling.

## Rules that matter here

- Tests never touch a real pane or arm a real timer. Use the stand-ins under
  `testdata`, temporary directories, and the mock installer.
- State files written by older versions must keep working. Add keys with
  defaults; do not rename or remove one.
- Text is sent once. Recovery may press Enter again on fresh evidence of a
  stuck draft, and never resends the message or answers a dialog.

[AGENTS.md](AGENTS.md) has the full list, for people and coding agents alike.

## Layout

| Location | Responsibility |
| --- | --- |
| `cmd/schedule-send` | Program entrypoint. |
| `internal/cli` | Arguments, picker, scheduling, and management commands. |
| `internal/target`, `limit`, `notice`, `when` | Target matching, usage-limit detection, and time parsing. |
| `internal/herdr`, `screen` | Herdr calls and terminal-screen interpretation. |
| `internal/job` | State files, locking, and cleanup, with each system's timers in its own file (`systemd_linux.go`, `launchd_darwin.go`). |
| `internal/installpath` | The recorded installation path and the path rules `tools/build` shares. |
| `internal/worker` | Delivery and submission verification. |
| `internal/show`, `term` | Text/JSON output and terminal handling. |
| `herdr-plugin.toml`, `plugin` | The Herdr plugin: one action that opens the scheduling prompt for the focused agent. |
| `.githooks`, `tools` | Automatic installation, builds, and verification. |
| `testdata` | Stand-in commands, test fixtures, the demo scenario, and the zsh baseline. |
| `docs/media` | The README's recordings, their tapes, and the diagram. |

## Installing from a checkout

A build only arms jobs from the path it records, so a checkout installs one of
its own. From the main checkout on `main`, run once:

```bash
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

```bash
tools/install
```

Installation builds `bin/schedule-send`, replaces it atomically, and links
`~/.local/bin/schedule-send` to it. Only one installation can run at a time.
Run `tools/install` from the main checkout on `main` and invoke the installed
command; never point the installed binary at a worktree. The zsh shim,
`schedule-send.zsh`, remains so timers created before the Go rewrite still
work.

Hook configuration is local to each repository; run `tools/setup-hooks` after
a fresh clone. Setup refuses to overwrite a different `core.hooksPath`.
To disable these hooks:

```bash
git config --local --unset core.hooksPath
```

## README media

The GIFs in `docs/media` are recorded from the real program by `tools/media`,
which runs each `docs/media/*.tape` with [VHS](https://github.com/charmbracelet/vhs)
in a container. It needs Docker and Go, works on Linux and on a Mac, and uses
the demo scenario in `testdata/demo` with the stand-ins, so nothing is
delivered and no timer is armed.

```bash
tools/media                  # every tape
tools/media resume           # one
```

`TestDemoScenario` pins what the hero recording shows, so a change to the
program's output fails the checks until the test is updated; record the GIFs
again in the same change. Each run gives slightly different bytes, so commit
the GIFs only when what they show has changed.

The first run pulls the VHS image and builds a small one on top of it, about
3 GB together. To remove them afterwards:

```bash
docker rmi schedule-send-media:v0.12.1 ghcr.io/charmbracelet/vhs:v0.12.1
```

## Commit messages

Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`) keep the
changelog easy to assemble, with a body that says why the change was needed.

## CI

Every push to `main` and every pull request runs the checks on Linux, Linux
ARM and macOS (`.github/workflows/ci.yml`). The macOS job also installs the
formula with Homebrew and arms real launchd jobs aimed at the stand-in Herdr.

## Releasing

Maintainers only. Add what changed under `## [Unreleased]` in `CHANGELOG.md`
as you go. To release, from an up-to-date `main`:

```bash
tools/release 1.2.3
```

It moves the Unreleased entries under a heading dated today, sets the
version in `herdr-plugin.toml`, commits, and tags `v1.2.3`. It pushes
nothing; it prints the push command. Pushing the tag runs the release
workflow, which checks that the tag, the changelog and the plugin manifest
agree, runs CI, publishes the GitHub release with the Linux archives, installs
it on x86-64 and ARM, bumps the formula in
[dj-skn/homebrew-tap](https://github.com/dj-skn/homebrew-tap), and installs
from the tap on macOS.

A version such as `1.2.3-rc.1` makes a pre-release, which is never sent to the
tap. The tap bump needs a `HOMEBREW_TAP_TOKEN` secret holding a token that can
write to the tap.
