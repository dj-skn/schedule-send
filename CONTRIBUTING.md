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

## Rules that matter here

- Tests never touch a real pane or arm a real timer. Use the stand-ins under
  `testdata`, temporary directories, and the mock installer.
- State files written by older versions must keep working. Add keys with
  defaults; do not rename or remove one.
- Text is sent once. Recovery may press Enter again on fresh evidence of a
  stuck draft, and never resends the message or answers a dialog.

[AGENTS.md](AGENTS.md) has the full list, for people and coding agents alike.

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
