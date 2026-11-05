# SS-4 — Cancel a queued send from the picker

## Status

The owner instructed the P1 build on 2026-11-05. Story branch `ss-4`
starts at `343e46f` on `main`. P1 is built and verified on
`ss-4-p1` and fast-forwarded into `ss-4`.

The story starts with Go 1.27.3 and Herdr 0.9.5. The Go release and
Homebrew's stable formula both report 1.27.3. Herdr's latest stable release
is 0.9.5, which matches the plugin manifest and README.
[Go releases](https://go.dev/dl/) · [Homebrew Go](https://formulae.brew.sh/formula/go) ·
[Herdr releases](https://github.com/herdrdev/herdr/releases/latest)

## Outcome

While choosing a target, the user can cancel one send that the picker marks
as queued. They see which send they are about to cancel, confirm explicitly,
and get a receipt or an error. The picker uses the same cancellation path
as `--cancel ID`.

## P1 — Select and cancel one send

### Code check

- The picker accepts a row number, with Enter taking row 1. It opens for
  multiple matching targets on a terminal. A single match and `--pane`
  skip it, including the focused-pane Herdr overlay. A dry run can still
  show the picker. [Target selection](../internal/cli/cli.go#L766) ·
  [Direct pane selection](../internal/cli/cli.go#L533)
- The queued scan matches readable state to the pane, agent and folder, then
  asks whether its timer is armed. It already handles the local calendar
  time in older state files. It retains only fire times, so the picker needs
  the original job ID to cancel a particular send.
  [Queued sends](../internal/cli/queued.go#L17)
- A job ID comes from the pane and fire time. Another run can cancel and
  re-arm the same ID while the confirmation is open. The state file is
  published by atomic rename under the per-job lock. Both scheduler backends
  take that lock before removal, so they can verify the selected state file
  there before stopping anything. [Job IDs](../internal/cli/queued.go#L129) ·
  [State publication](../internal/job/job.go#L280) ·
  [Linux removal](../internal/job/systemd_linux.go#L351) ·
  [macOS removal](../internal/job/launchd_darwin.go#L288)
- The scheduling flow hands Ctrl-C back before it mutates state. The picker
  can use the same boundary before cancelling. A worker can already be
  delivering when cancellation starts, so the receipt describes what the
  scheduler stopped and removed. [Interrupt handling](../internal/cli/cli.go#L165) ·
  [Cancellation command](../internal/cli/cli.go#L1159) ·
  [Job management](usage.md#manage-jobs-and-missed-runs)

### Decisions settled 2026-11-05

1. **Entry.** Recommend `c N` at the existing picker, where `N` is a
   queued pane's row number. Show a numbered list only when the pane has
   several sends. This keeps number and Enter selection familiar. A separate
   management menu would take another step and could later expose more actions.
2. **Confirmation.** Recommend showing the target, local date and time, and
   job ID, then asking for an explicit `y`. Enter and `n` go back. A
   message or keys preview would add context, with extra work to read payloads
   safely and fit narrow terminals.
3. **After cancellation.** Recommend printing the receipt and ending the
   command. That gives the invocation one effect. Returning to the picker
   would let the user schedule another send immediately and would require
   refreshing rows, queued marks and interrupt handling after a mutation.

### Assumptions for the build

- Offer the action only when the existing interactive picker has a queued
  row and the run uses human output with writes enabled. Dry runs, JSON,
  and non-interactive calls keep their current behavior.
- Only the selected send can be cancelled. If it disappears or is replaced
  while the user is deciding, say it changed and require a fresh selection.
  Surface scheduler errors using the current cancellation semantics.

### Steps

1. Retain the stored job ID and fire time for each matching armed send.
   Preserve queued counts, ordering and legacy calendar times.
   → verify: focused CLI tests cover several jobs, older state, a reused
   pane, invalid state and a timer that is gone.
2. Add a guarded path to the existing removal code. Read the selected
   state for the confirmation and hold its file open across the prompt.
   Take the normal removal lock after confirmation, then compare the
   open file's identity with the current state path before disarming.
   → verify: replacing the state with a new job at the same ID leaves that
   job and its timer intact. A missing job, busy lock and failed disarm
   each produce an honest result. Existing `--cancel` behavior passes.
3. Add the picker action, exact-job selection, confirmation and early
   return from the scheduling flow after success. Reuse the existing
   cancellation receipt. → verify: a terminal fixture with two sends on
   one pane and another pane's send cancels only the chosen job and arms
   nothing. Check Back, Ctrl-C, EOF, invalid input, 52-column output,
   dry runs, JSON and non-terminal behavior.
4. Update the usage reference and Unreleased changelog. Review the terminal
   flow using temporary state and the repository stand-ins.
   → verify: `tools/check` and full Linux `tools/parity` pass, with the
   picker and cancellation cases checked first. macOS CI exercises the
   launchd stand-in when the owner authorizes a push. Record results in
   As built and fast-forward P1 into `ss-4` after verification.

Estimated size is one to two working days. Deferred scope includes replacing
or rescheduling jobs, bulk cancellation, a plugin action, and surfacing the
action when only one target matches. These are separate entry points or
operations from the queued picker action.

The owner's hand check uses temporary state and stand-ins. From a numbered
picker with several queued sends, decline one confirmation, then confirm
another. Compare the remaining jobs with the initial list.

### As built — 2026-11-05

- Type `c N` at the interactive picker. A pane with several sends shows
  them in time order with their IDs. Confirmation shows the pane, local
  date and time, and ID. Enter or `n` draws the picker again. `y` removes
  the chosen send, prints the ordinary cancellation receipt and ends the
  invocation.
- The state file stays open through confirmation. Cancellation compares
  its identity under the same lock as ordinary `--cancel` before
  contacting the scheduler. A send that went away or was replaced gets
  a message to open the picker again. A failed disarm reports the error.
- `tools/check` passed all 12 Go test packages, both platform vet passes,
  and the script checks. The focused picker and cancellation parity run
  had 3 identical cases, 5 known differences and zero unexpected
  differences. Full Linux parity had 39 identical cases, 39 known differences
  and zero unexpected differences.
- The focused regression tests covered exact selection among several jobs,
  invalid choices, decline, EOF, dry run, JSON, a non-terminal call,
  same-ID replacement, a busy lock and failed disarm. A 52-column Linux
  terminal run with stand-ins confirmed that Ctrl-C exits 130 without
  changing the queue and `y` stops only the selected job and arms nothing.
- Native macOS verification remains for CI. The owner can check the
  terminal interaction using the stand-in steps above.
