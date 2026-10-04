# Repository agent guidance

## Project

`schedule-send` schedules text or key presses into a running Herdr agent or
pane, through on-disk `systemd --user` timers. Linux only. See `README.md`.

## Commands

```bash
tools/check      # the verification gate: gofmt, go vet, go test
tools/install    # build bin/schedule-send and link it into ~/.local/bin
```

## Rules

- `~/.local/bin/schedule-send` and every armed job run from the main checkout,
  `/home/dev/src/schedule-send`. Keep it on `master`; build feature work in
  worktrees under `/home/dev/src/schedule-send.worktrees/`, and run
  `tools/install` only from the main checkout.
- Never arm a timer against a real pane from a test. Tests use a stand-in herdr.
- A job's state file (`~/.local/state/herdr-send/<id>.json`) is read by
  whatever version is installed when it fires. Add keys with defaults; never
  rename or remove one.

## Stories

Stories use the prefix `SS`, giving `SS-1`. Plans live in
`docs/ss-<n>-<slug>-plan.md`; branches are `ss-<n>` with phase branches
`ss-<n>-p<k>` off it.
