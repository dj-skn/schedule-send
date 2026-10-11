# SS-2 — Delivery and scheduling reliability

## Status

Plan recorded on 2026-10-11 at the owner's request, on story branch `ss-2`,
starting from `master` at `034c9de`. No implementation phase is approved,
built, verified, merged, or installed. Nothing is pushed.

The owner requested a saved implementation plan following the code analysis.
That authorizes this document and its story-branch commit, not implementation.
Recommendations below remain proposed until the owner approves them.

## Outcome

Recovery must not answer a dialog that appears during focusing. New timers
must use the installed main-checkout executable, with a correctly encoded
systemd command. Malformed numeric state must fail before delivery, while old
files retain their optional defaults. Status inspection must finish within a
bounded time and explain failures in both text and JSON output.

## Evidence and baseline

The initial analysis ran against the starting master revision on 2026-10-11:

- `tools/check`: exit 0; all 11 tested Go packages passed, along with vet,
  formatting, shim/shell syntax, and isolated hook checks.
- Full `tools/parity`: exit 0; 41 identical cases, 37 documented differences,
  and zero unexpected differences.
- These are baseline results from the analysis, not verification of future
  phases.
- Planning-commit check on 2026-10-11: `env
  GOCACHE=/tmp/herdr-ss-2-go-cache tools/check` exited 0; all 11 tested Go
  packages and hook/syntax checks passed. The first attempt could not write
  the normal Go cache under sandbox permissions; it was an environment
  failure, resolved with the temporary cache. Parity was not rerun for this
  documentation-only change.
- Tests and parity use temporary state and stand-in commands. No real panes
  were targeted or real timers armed.

Source findings were re-read when recording this plan:

| Finding | Evidence | Work it requires |
| --- | --- | --- |
| Recovery checks blocked state before focus, but not after focus when the draft is still visible | `internal/worker/deliver.go`, `recoverSubmission` | Check for a dialog immediately before each recovery Enter, including after focus |
| New services use the currently running executable | `internal/cli/cli.go`, `selfPath` and service publication | Establish an installed executable identity before arming |
| ExecStart interpolates the executable without escaping | `internal/job/job.go`, `ServiceUnit` | Encode the executable for systemd syntax |
| Invalid numeric fields silently take defaults | `internal/job/job.go`, `Load` and `whole` | Distinguish omitted legacy values from malformed supplied values |
| Status journal reads have no deadline and discard errors | `internal/cli/cli.go`, `status` and `statusJSON` | Bound journal reads and expose command diagnostics |

These are inspection findings. Specific failure cases need regression tests
before the fixes are implemented. The existing SS-1 document records its
history; its old status paragraph is not the current master behavior.

## Proposed decisions

1. **Recovery:** keep the existing ladder and one text send. Require fresh
   draft evidence and a non-blocked lifecycle immediately before every Enter.
   Restore previous agent focus on each exit after focusing. Do not infer a
   successful submit from a read failure or retry text.
2. **Installed executable:** recommend embedding the stable main-checkout
   binary path during `tools/install`, together with an installation identity
   that can be checked before arming. Reject real scheduling by unmanaged
   builds, with an actionable message; retain dry-run and stand-in testing.
   Reconfirm the exact metadata and validation design during P2 orientation.
   This avoids hardcoding this user's home directory or discovering the Git
   checkout on every CLI invocation. The alternative is runtime discovery of
   the installed command; it requires stronger protection against stale links
   and worktree executables and should be compared if embedding proves awkward.
3. **State compatibility:** preserve existing field names and defaults for
   absent optional numeric fields. Recommend treating null optional numeric
   fields like absence for legacy compatibility; reject strings, booleans,
   negative/fractional numbers, overflow, and invalid control ranges. Decode
   integers without float64 conversion. Validate `run_at` as an int64 with zero
   retained as the old-file default.
4. **Status errors:** keep missing-state status inspection usable for completed
   jobs and their journals. Preserve the existing JSON result fields; add
   structured diagnostics for command failures and warnings in text mode.
   Return a failure for inability to collect diagnostics, but distinguish a
   normal systemctl inactive/failed-unit response from a transport failure.
   Set a 10-second journal deadline plus bounded pipe draining. Confirm the
   systemctl result contract before implementing exit-code changes.
5. **Scope:** defer the CLI file split. Mixing file moves with behavioral fixes
   would make regressions harder to trace. No new scheduling features belong
   in this story.

## Phases

| Phase | Outcome | Branch | Status |
| --- | --- | --- | --- |
| P1 | Recovery stops when a dialog appears during focusing | `ss-2-p1` | planned; not approved |
| P2 | Stable installed executable and valid systemd ExecStart | `ss-2-p2` | planned; not approved |
| P3 | Strict numeric state loading with legacy defaults | `ss-2-p3` | planned; not approved |
| P4 | Bounded, honest status diagnostics | `ss-2-p4` | planned; not approved |

Each phase starts with a fresh source check and an owner walkthrough. Build
only the approved phase on its phase branch. Every implementation step has a
conventional commit with a body explaining why, after the required gate.
Record As built and verification results, then fast-forward the phase into
`ss-2`. Stop if a fast-forward cannot be made. A master merge and push require
separate owner instructions. Installation is only from clean main master;
repository hooks may install program changes after an authorized master merge.

## P1 — Recovery stops at dialogs

### Steps

1. Extend the fake worker with a focus callback or equivalent controlled
   transition. Cover a dialog appearing during focus while the draft remains
   visible, screen failure after focus, and a draft submitted during focus.
   Assert exact prompt/key counts and restoration of prior focus.
   → verify: the dialog regression fails against the starting implementation;
   existing worker tests retain their outcomes.
2. Check blocked lifecycle after the final draft read and before each recovery
   Enter. Share only the guard needed to keep both ladder rungs consistent.
   Preserve unknown-screen behavior and the existing recovery budget.
   → verify: worker tests show one text send, no Enter after a dialog, no Enter
   on unreadable evidence, and focus restoration on all relevant exits.
3. Record behavior and As built, including the unavoidable interval between
   checking state and issuing an external command.
   → verify: `tools/check`; parity cases `fire-resume`, `fire-old-state`,
   `fire-draft-stuck`, `fire-draft-focus`, `fire-draft-none`, `fire-refused`.

Not included: redesigning Herdr, making screen/state reads atomic, changing
initial prompt policy for blocked agents, or automatic rescheduling.
Estimated size: one small implementation phase, roughly half a day to one day.

### As built

Not built.

## P2 — Stable executable and systemd command encoding

### Steps

1. Inspect installer, shim, source hashing, hooks, and the CLI test injection.
   Settle how installation metadata identifies the stable executable and how
   test builds inject it without exposing a production bypass flag.
   → verify: a design covering installed invocation, worktree/go-run invocation,
   moved/missing installations, symlink invocation, and legacy shim timers.
2. Make installer-built binaries record their stable main binary path. Validate
   it before publication/arming; unmanaged real scheduling must fail before
   creating job artifacts. Keep preview and management commands usable.
   → verify: isolated installer/hook and CLI tests for stable path selection,
   unmanaged-build refusal, dry-run, and no files on early failure.
3. Encode ExecStart using systemd's quoting and specifier rules, not shell
   escaping. Include spaces, quotes, backslashes, and percent characters in
   unit tests. Reject line-breaking metadata in descriptions if needed.
   → verify: exact unit tests and, when available, offline systemd-analyze
   verification using temporary units; never load them into the user manager.
4. Update README and parity expectations narrowly for intentional path changes.
   Keep old service paths and the zsh shim functional; do not rewrite armed jobs.
   → verify: `tools/check` and full `tools/parity`, with no unexplained differences.

Not included: migrating existing timers, installation from worktrees, or adding
Git as a runtime dependency. Old timers keep their previously recorded paths.
Estimated size: one to two days; installer/runtime contract is the main cost.

### As built

Not built.

## P3 — Strict numeric state loading

### Steps

1. Add table-driven loading cases for force, quiet, grace, and run_at: absent,
   null, valid integer, string, boolean, negative, fractional, and overflow.
   Include original old-state fixtures and control range validation.
   → verify: malformed supplied values reproduce the silent-default gap;
   missing legacy fields still load with documented defaults.
2. Replace float64 coercion with explicit integer decoding and field-specific
   errors. Keep int64 epochs separate from small control integers. Confirm
   invalid state is rejected before Herdr delivery begins.
   → verify: job and worker tests, including zero prompt/key/run calls for
   malformed state and unchanged old-state delivery behavior.
3. Record the compatibility contract and As built.
   → verify: `tools/check`; parity `fire-old-state`, `fire-force`, `fire-keys`,
   `fire-resume`, `list`, and `status`.

Not included: state schema renames, cleanup of malformed jobs, or changing
optional string-field compatibility.
Estimated size: half a day to one day.

### As built

Not built.

## P4 — Bounded status inspection

### Steps

1. Separate status collection from rendering enough to share bounded command
   execution and results. Read the systemctl contract and existing fixtures
   before deciding which nonzero statuses mean inspection failed.
   → verify: stand-ins distinguish inactive/failed units, missing state, bus
   failure, missing journalctl, command failure, and deadline expiry.
2. Add journal deadlines and bounded pipe draining; expose partial output and
   diagnostics in text and additive JSON fields. Propagate collection failures
   through the CLI without losing useful completed-job journal inspection.
   → verify: CLI tests assert bounded completion, exactly one JSON result,
   structured errors, partial output, and the agreed exit-code behavior.
3. Update README and parity for explicitly approved output differences. Record
   As built and complete the story status only after all four phases are verified.
   → verify: `tools/check` and full `tools/parity`; inspect text and JSON
   stand-in output by hand.

Not included: streaming journal follow, persistent result history, or a general
CLI file reorganization.
Estimated size: one day.

### As built

Not built.

## Owner checks and delivery

Review regression transcripts, generated service files, and text/JSON status
fixtures produced in temporary directories. All acceptance exercises use the
stand-ins in `testdata`; do not test delivery against real panes or timers.
Each report states what was checked now, any changed compatibility behavior,
and what still needs owner review.

Once every phase is built, verified, and fast-forwarded into `ss-2`, present
its final diff and verification results for the owner's master-merge decision.
Do not push, merge master, run tools/install, or run tools/setup-hooks as part
of preparing or building this plan.
