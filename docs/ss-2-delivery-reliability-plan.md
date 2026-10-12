# SS-2 — Delivery and scheduling reliability

## Status

Plan recorded on 2026-10-11 on story branch `ss-2`, starting from `master`
at `034c9de`. P1–P3 are complete in `ss-2`. P4 is built and verified on
`ss-2-p4`; this update accompanies its fast-forward into `ss-2`. All four
phases are complete after that fast-forward. Nothing is installed, merged
into master, or pushed. Fixes from the PR #1 review are recorded at the end.

The owner approved and instructed builds for P1, P2, and P3 on 2026-10-11.
The owner also agreed to the P4 recommendations and instructed build on
2026-10-11. All four phases now have build approval, scoped to each phase.

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
| P1 | Recovery stops when a dialog appears during focusing | `ss-2-p1` | complete in ss-2 |
| P2 | Stable installed executable and valid systemd ExecStart | `ss-2-p2` | complete in ss-2 |
| P3 | Strict numeric state loading with legacy defaults | `ss-2-p3` | complete in ss-2 |
| P4 | Bounded, honest status diagnostics | `ss-2-p4` | complete in ss-2 |

Each phase starts with a fresh source check and an owner walkthrough. Build
only the approved phase on its phase branch. Every implementation step has a
conventional commit with a body explaining why, after the required gate.
Record As built and verification results, then fast-forward the phase into
`ss-2`. Stop if a fast-forward cannot be made. A main merge and push require
separate owner instructions. Installation is only from clean main;
repository hooks may install program changes after an authorized main merge.

## P1 — Recovery stops at dialogs

### Decisions settled 2026-10-11

The owner agreed to the walkthrough decisions and explicitly instructed build.
Keep this phase limited to recovery, leaving initial prompt policy unchanged.
Preserve unknown lifecycle behavior: fresh draft evidence may authorize recovery
unless the lifecycle reports blocked. Require the dialog check after the final
screen read and before every recovery Enter, including the focus rung.

The source recheck confirmed the existing pre-focus blocked check and its missing
post-focus counterpart. Existing tests cover already-blocked agents and normal
focus restoration, but do not simulate a dialog opening during focus. Add a
controlled focus callback to exercise those transitions.

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

### As built — 2026-10-11

- `recoverSubmission` retains the existing pre-focus dialog check and adds a
  final blocked-lifecycle check immediately before every recovery Enter.
  A focus-time dialog with a visible draft returns the existing dialog failure
  and restores the previously focused agent without another Enter.
- Four focus-transition regression cases cover a dialog with a retained draft,
  an unreadable screen, submission during focus, and unknown lifecycle with
  fresh draft evidence. Each asserts exact prompt/key/focus calls and previous
  focus restoration. The dialog case also checks the original draft remains.
- The corrected regression fixtures were run against the old code: only the
  dialog case failed, reporting exit 0 and a focused/Enter-retried success
  where exit 1 was expected. The other three cases passed. An initial trailing
  space mismatch in the unreadable-screen expectation was corrected first.
- The implementation adds six production lines. Tests and fix share commit
  `baf24bb`: committing the intentionally failing regression alone would violate
  the repository's requirement to pass `tools/check` before every commit.
- Verification run now: `env GOCACHE=/tmp/herdr-ss-2-go-cache tools/check`
  exited 0, with all 11 tested Go packages plus formatting, vet, shim/shell
  syntax, and hook checks passing. The worker suite includes 29 top-level
  tests, with four new subcases in the new test.
- The six planned parity cases exited 0: zero identical cases, six documented
  baseline differences, and zero unexpected differences. No parity expectation
  changes were needed. This is P1 verification, not a full-story parity run.
- Check by hand through stand-ins: inspect the transition test's exact command
  assertions and failure output; inspect normal recovery/focus parity output.
  No real pane or timer test is required or permitted.
- The interval between the last lifecycle check and Herdr receiving Enter
  remains non-atomic. Unknown lifecycle behavior and initial prompt policy are
  unchanged. No state fields, timer files, or installer behavior changed.
- Later phases inherit this recovery guard; P2 can proceed independently once
  its walkthrough and design decisions are approved.


## P2 — Stable executable and systemd command encoding

### Decisions settled 2026-10-11

The owner agreed to the P2 walkthrough and explicitly instructed build.
Embed the final absolute executable path during installation, using encoded
metadata to avoid build argument quoting problems. Require real scheduling to
run at that resolved path and validate it before publishing any artifacts.
Unmanaged, copied, moved, and go-run binaries cannot arm jobs. Keep dry-run,
management, and legacy internal firing usable. Preserve old units and the shim.
Encode new ExecStart paths with systemd quoting and specifier rules; reject
unsupported control characters before publication.

A small shared build helper will supply encoded metadata for the installer and
isolated parity fixtures. It builds only; it does not install or link anything.
Installer/hook tests continue using mock installers. The helper must be included
in source hashing and syntax checks so its changes trigger automatic rebuilding.

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

### As built — 2026-10-11

- `tools/build OUTPUT INSTALLED_PATH` encodes the final path with GNU base64
  and passes it as linker metadata. `tools/install` uses the stable final
  main-checkout `bin/schedule-send` path, not its temporary output filename.
  Atomic publication, locking, branch restrictions, and the zsh shim remain.
- `internal/installpath` validates decoded clean absolute UTF-8 paths, rejects
  control characters, checks executable mode, resolves symlinks, and compares
  the running executable location with the recorded installation location.
  Plain builds, copied executables, and moved/missing installations fail before
  scheduling creates job files. This is location metadata, not a cryptographic
  installation attestation; tests intentionally build temporary installations.
- The CLI uses the recorded path for services. Dry-run, management operations,
  and `__fire` bypass the arming guard. Existing units and state are not migrated.
  Tests inject the resolver privately; there is no runtime environment override.
- **Correction to the anticipated command encoding:** offline systemd checks
  rejected direct executable tokens containing quotes, apostrophes, or
  backslashes even when quoted/escaped. New units use
  `ExecStart=:/usr/bin/env -- "<escaped installed path>" __fire <id>` instead.
  env executes that path as an argument. The colon disables environment
  expansion, preserving dollar signs; percent specifiers and C-style quoting
  are escaped. Service metadata controls and invalid UTF-8 fail before locking
  or publication. `/usr/bin/env` is checked before real scheduling.
- Build requirements now include GNU coreutils base64/tr; new timer services
  require `/usr/bin/env`. README and shared repository guidance record this.
- Build helper changes participate in source hashing and shell syntax checks.
  Mock hook tests prove changing that helper triggers rebuilding, increasing
  fixture installation count from six to seven without touching the real install.
- Tests cover 12 identity table cases plus nonexecutable/moved paths, exact build
  arguments and four invalid helper paths, six unusual executable encodings,
  six invalid service metadata cases, and early CLI refusal without artifacts.
  A compiled fixture exercises staging-to-final publication, moved/copied/plain
  builds, dry-run and management, unmanaged worker dispatch, symlink invocation,
  and exact generated service contents using Herdr/systemd stand-ins.
- Verification run now: `env GOCACHE=/tmp/herdr-ss-2-go-cache tools/check`
  exited 0; all 12 tested Go packages plus formatting, vet, shim/shell syntax,
  and isolated hook checks passed. Full `tools/parity` exited 0: 41 identical,
  37 documented differences, zero unexpected differences. Its strict arm-case
  comparison asserts the entire new ExecStart line before comparing remaining
  artifacts; it does not broadly ignore unit differences.
- Offline `systemd-analyze --user verify` exited 0 on a temporary service made
  by the actual renderer with spaces, dollar/percent signs, double/single quotes,
  and a backslash in its executable path. Nothing was loaded or started.
  Offline verification required sandbox escalation for local socket setup; a
  sandboxed attempt failed with Operation not permitted, not a unit parse error.
- During development, an old CLI assertion failed because it expected unquoted
  ExecStart; it was updated explicitly. A compiled fixture then checked all of
  XDG_CONFIG_HOME for absence, but Go compilation creates its own configuration
  there. The assertion now checks the actual job-state and systemd-unit paths.
  Both failures were understood and corrected before final verification.
- Implementation commits: `0e7e1e0` (build metadata/tooling) and `d32c733`
  (arming guard, unit rendering, tests, docs, and strict parity update).
- Owner review uses the identity/compiled-fixture tests and generated temporary
  units. Real panes/timers and the actual installer were not exercised. Main
  master and its installed binary remain unchanged. Installation remains a
  later master-merge operation; no manual live scheduling check is required.


## P3 — Strict numeric state loading

### Decisions settled 2026-10-11

The owner approved all three walkthrough recommendations and instructed build.
Absent and null numeric fields retain force=0, quiet=0, grace=4, and run_at=0.
Accept JSON integer literals only; reject decimal/exponent notation even when
mathematically integral. Reject invalid types, negatives, overflow, force/quiet
outside 0–1, and grace outside 1–60 during Load with field-specific errors.
Decode run_at directly as a nonnegative int64. Keep State.Validate as a second
guard. Do not delete malformed jobs or change optional string defaults.

Source orientation confirmed an existing test explicitly expecting malformed
grace values to default, and float64/int coercion for run_at. Worker.Run already
returns before connecting to Herdr when Load fails, so production worker changes
should not be needed. Any low-level test fixture that explicitly encodes grace=0
must be corrected to a valid value rather than weakening the approved range.

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

### As built — 2026-10-11

- Load decodes raw JSON values directly as int64 integers. It rejects decimal
  and exponent forms, malformed types, overflow/underflow, negatives, and
  out-of-range controls with the offending field named. No float64 or
  platform-dependent int conversion is used for run_at.
- Absent and null fields retain force=0, quiet=0, grace=4, and run_at=0.
  Optional string defaults and existing field names remain. State.Validate
  continues checking controls and now also rejects negative firing epochs.
- The loader regression failed against the old code: malformed values were
  accepted, run_at=9007199254740993 became 9007199254740992, and the maximum
  int64 became zero. A corrected worker fixture with grace=-1 also failed
  against the old loader: it sent the prompt and exited 0 instead of refusing.
- The first worker reproduction fixture accidentally re-encoded local state
  without the stored Herdr executable, so it failed at binary discovery rather
  than proving delivery. It now mutates the actual armed state file. The old
  loader was temporarily restored only for the focused reproduction and the
  corrected loader was restored immediately afterward before final checks.
- Coverage: 56 basic field cases plus six range/exact-epoch cases; 36 worker
  refusals across agent text, agent keys, and pane text; two list/status
  inspection cases. Worker refusals assert zero connections, Herdr actions,
  and systemctl actions, with state bytes and job artifacts retained.
- List warns and skips malformed numeric state. Status exposes the original
  stored JSON, including the invalid 4.0 notation, without changing it.
- One low-level concurrent-write fixture and two management fixtures explicitly
  encoded grace=0 through zero-valued State structs. They now encode grace=4;
  the missing-field compatibility tests still test genuinely absent fields.
  The old default test no longer endorses -1 or 4.5; explicit null still defaults.
- No production worker or management code changes were needed: existing Load
  failure handling already stops before connecting and retains state.
  README records the supported numeric syntax, ranges, and defaults.
- Verification run now: `env GOCACHE=/tmp/herdr-ss-2-go-cache tools/check`
  exited 0, with all 12 tested Go packages plus formatting, vet, shim/shell
  syntax, and isolated hook checks passing. The six planned parity cases
  exited 0: two identical, four documented differences, zero unexpected.
  This is targeted P3 parity, not a new full-story parity run.
- Tests and fix share one implementation commit so no intentionally failing
  regression is committed, as tools/check must pass before every commit.
  Owner review can inspect the numeric table tests and the list/status fixture
  output; no real-pane or real-timer check is required or permitted.
- P4 inherits strict loading and retained malformed-state inspection. No job
  schema migration, installation, master merge, or push was performed.


## P4 — Bounded status inspection

### Decisions settled 2026-10-11

The owner agreed to all four walkthrough recommendations and instructed build.
Use a shared 30-second budget for timer status plus any structured unit-state
query, followed by a separate 10-second journal deadline and bounded pipe drains.
Inactive, failed, and missing units are successful inspection when confirmed
by their returned unit state; missing state files are normal for completed jobs.
Missing commands, deadlines, transport failures, unreadable state, and invalid
JSON cause exit 1 while retaining partial results. Malformed numeric state is
still inspectable when its JSON is valid. Preserve state/timer/journal/warning
JSON fields and add optional structured diagnostics. Emit exactly one stdout
result even when collection fails, plus the existing stderr error envelope.

Source orientation confirmed journalctl is unbounded and command errors are
ignored in both modes. The installed systemctl manual maps status exit 3 to
inactive and 4 to no unit but warns that exit codes alone are imperfect. Query
LoadState/ActiveState/SubState when a nonzero status could represent a normal
unit condition. Other commands and successful status output need no extra read.
A status-specific collector file avoids duplicating collection without a broad
CLI refactor. All status tests must shadow journalctl as well as systemctl.

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

- Both renderers now share internal/cli/status.go. Timer status and its
  conditional property query share 30 seconds; journal reads have 10 seconds;
  each command pipe drain has a one-second limit. The budget is for external
  commands; ordinary state-file reads remain filesystem operations.
- Exit 3/4 triggers a LoadState/ActiveState/SubState query. Confirmed inactive,
  failed, or missing units remain successful observations. Missing state still
  has the original completed-job warning. A unit may transition between the
  two reads, so a confirmed active state is also a normal observation.
- Collection failures return exit 1 after rendering partial results. JSON keeps
  the original fields and optionally includes source/severity/code/message,
  command exit code, stderr, and partial output in diagnostics. It emits one
  stdout object and the existing failed error envelope on stderr. Successful
  commands with stderr produce warnings without changing the exit code.
- Invalid JSON and unreadable/nonregular state paths are errors; valid stored
  JSON remains inspectable even with invalid numeric controls. No strict job
  loader is used for status inspection. Nonregular paths are refused before
  reading so a FIFO cannot block inspection.
- The common harness now shadows journalctl as well as systemctl. New status
  fixtures restrict PATH to their temporary command directory; missing-command
  cases cannot fall through to real commands. No timers are armed or panes used.
- Coverage: 16 scenarios in both text and JSON (32 cases), plus four deadline
  cases for timer, journal, the shared timer/query budget, and inherited pipes.
  The shared-budget fixture would succeed if the query received a fresh budget.
  Deadline fixtures completed in about 82 ms with an 80 ms budget; inherited
  pipe draining completed in about 23 ms with a 20 ms limit. Partial timer and
  journal output survives each failure.
- Steps 1 and 2 are one implementation commit because collection, error
  propagation, and deadline regressions need to pass together. README documents
  budgets, normal observations, failures, and additive diagnostics. Existing
  status parity cases remain identical; no parity exception was added.
- Fixture-based text and JSON inspection used an isolated CLI build and
  temporary state/commands. A journal exit 2 retained timer-partial,
  journal-partial, and raw grace=4.0; text named journal and JSON included its
  exit code, stderr, partial output, and separate failed stderr envelope.
  Review outputs are in /tmp/herdr-ss-2-status-review-4qblx8r3 (temporary).
- Verification: tools/check passed all 12 tested Go packages, formatting,
  vet, shim/shell syntax, and isolated hook checks. Full tools/parity exited 0:
  41 identical, 37 documented differences, zero unexpected. These are final
  full-story checks run now, rather than earlier-phase results. No live
  systemd or delivery check is needed.
- All four phases are built. Master merge, push, and installation remain outside
  this phase's authorization; the story is ready for the owner's merge decision.


## Owner checks and delivery

Review regression transcripts, generated service files, and text/JSON status
fixtures produced in temporary directories. All acceptance exercises use the
stand-ins in `testdata`; do not test delivery against real panes or timers.
Each report states what was checked now, any changed compatibility behavior,
and what still needs owner review.

Once every phase is built, verified, and fast-forwarded into `ss-2`, present
its final diff and verification results for the owner's main-merge decision.
Do not push, merge main, run tools/install, or run tools/setup-hooks as part
of preparing or building this plan.

## Default branch rename — 2026-10-12

The owner requested renaming master to main after the private GitHub repository
and PR #1 were created. The GitHub default and local main-checkout branch now
use main. Installer, hook setup, automatic installation guards, and isolated
hook fixtures are updated on ss-2; README and AGENTS.md use main. Earlier
master references above describe the historical phase builds. The phase table
now reflects the completed story. No story merge or installation is included.

## Review fixes — 2026-10-12

The owner asked for every finding from the PR #1 review to be fixed, with
main as the project's branch. Built on top of ss-2 in six commits:

- **Deploy branch.** `tools/main-checkout` is now the only place that names
  main; install, hook setup and automatic installation call it. A main
  checkout still on master gets a `git branch -m master main` hint instead of
  silently skipping. The Go refusal message no longer names a branch.
- **ExecStart.** `installpath.Check` is the one rule set for recorded paths and
  now rejects `=`, which GNU env reads as an assignment even after `--`
  (reproduced with env 9.4). `tools/build` runs it through a small command and
  resolves OUTPUT relative to the caller. ServiceUnit escapes `$` as `$$`
  instead of using the `:` prefix, which needs systemd 249. The recorded path
  must be a regular file, not a symlink.
- **Recovery Enter.** A failed `agent get` now returns `herdr.Unreadable`
  rather than "unknown", and the dialog check stops the ladder on it. A
  reported "unknown" status still retries. The keys rung checks once.
- **Status.** Exit-3 stderr is a warning once show confirms the unit state,
  and a not-found failed unit is accepted. Status runs commands through
  `job.Run` and `Systemctl.Capture`.
- **Dry run.** would_run names the installed executable; a build that cannot
  arm warns and leaves it empty. It names the program and arguments rather
  than the env wrapper, so dry-run parity with the zsh baseline is unchanged.
- **Malformed state.** One table holds the numeric bounds for Load and
  Validate. Load returns the fields it decoded with its error. The worker
  notifies through Herdr (honouring a quiet flag that loaded) and keeps the
  files; `--list` shows the job as `invalid` with an additive JSON `error`.

Two earlier P3/P4 outcomes change on the owner's instruction: refused state
now raises a notification (P3 had zero Herdr connections) and is listed as
invalid (P3 skipped it), and exit-3 stderr is a warning (P4 treated it as a
transport failure). Rejecting decimal notation and never deleting or
delivering malformed jobs are unchanged.

Verification: `tools/check` exit 0, with all 12 tested Go packages, vet,
formatting, shim/shell syntax and hook checks. Full `tools/parity` exit 0: 41
identical, 37 documented, 0 unexpected, matching the ss-2 head run the same
way. The `zsh-baseline` tag is not on GitHub, so parity ran against the last
full zsh script ("feat: schedule a resume from the agent's usage-limit
notice") with `LANG=C.UTF-8`; under the C locale zsh truncates multibyte
excerpts by bytes and three fire-draft cases differ on the ss-2 head too.
`systemd-analyze verify` (systemd 255) accepts the new ExecStart; `$$`
unescaping at fire time and pre-249 systemd were not exercised, since the
container has no systemd manager. Nothing is installed or merged into main.
