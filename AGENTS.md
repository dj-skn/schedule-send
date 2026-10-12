# Repository guidance

## Project and entrypoints

`schedule-send` is a Linux Go CLI for scheduling messages or keys into Herdr
agents/panes through on-disk systemd user timers. Read [README.md](README.md)
for usage, dependencies, and the package map. `cmd/schedule-send` is the
entrypoint; `schedule-send.zsh` is the compatibility shim for older timers.

## Workflow

- Keep the main checkout on `main`. Develop and build feature changes in
  linked worktrees, kept in a `<checkout>.worktrees` directory beside it.
- Run `tools/check` before committing. For delivery or scheduling changes,
  run the relevant `tools/parity` cases or the full suite as appropriate.
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
| `tools/check` | Formatting, vet, Go tests, zsh shim syntax, shell syntax, and isolated hook tests. |
| `tools/parity [CASE...]` | Compare behavior against the zsh baseline in `testdata/baseline` with documented differences checked. |
| `tools/install` | Build, atomically publish, and link the binary; main checkout on `main` only. |
| `tools/setup-hooks` | Enable this repository's hooks and install if needed; main checkout on `main` only. |

Build/install needs Go 1.27.1+, Git, `flock`, and GNU coreutils (`base64`, `tr`). Checks also need `zsh` and
`jq`; parity uses Python 3 and `script`. Runtime needs Herdr, GNU `date`, `/usr/bin/env` (GNU coreutils), and
`systemctl --user`.

## Compatibility and delivery invariants

- Never test against real panes or arm real timers. Use the stand-ins under
  `testdata`, temporary directories, and mock installers for hook tests.
- Armed jobs run the installed main-checkout binary. Preserve the zsh shim and
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
  Keep JSON diagnostics structured and older optional state defaults intact.
- Keep `testdata/baseline/schedule-send.zsh`, the zsh script the program was
  ported from, unchanged for parity.

## Stories

Use story names such as `SS-1`, plans in `docs/ss-<n>-<slug>-plan.md`, and
branches `ss-<n>` with optional phase branches `ss-<n>-p<k>`. Plans record the
story's history; current behavior is defined by code, tests, and README.
