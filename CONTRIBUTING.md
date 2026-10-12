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
