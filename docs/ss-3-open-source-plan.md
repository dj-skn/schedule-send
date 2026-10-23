# SS-3 — Open-source release for Linux and macOS

## Status

Plan recorded on 2026-10-12 on story branch `ss-3`, starting from `main`. P1,
P2, and P3 are complete in `ss-3`; this update accompanies P3's fast-forward
from `ss-3-p3`, after the owner's Mac confirmed it. P4 is complete in `ss-3`
as well, fast-forwarded from `ss-3-p4` after the same confirmation. P5 is complete in
`ss-3` too, fast-forwarded from `ss-3-p5` after the owner's Mac run and a live
check in their Herdr. P6 is complete in `ss-3`, fast-forwarded from `ss-3-p6`
after a rehearsal on the private repository. P7 is complete in `ss-3`,
fast-forwarded from `ss-3-p7` after the owner's Mac run; its plugin
screenshot moved to P7.1. P7.1, which reworks the overlay's prompt, is
complete in `ss-3`, fast-forwarded from `ss-3-p7.1` after a Linux run on the
owner's homelab server and their live check in Herdr. P8 is in progress: its
record is on `ss-3`, and on 2026-10-23 it revised the story's decisions on the
history and the earlier plans. `ss-3-p3` to `ss-3-p7.1` were pushed to the
private repository for those runs, on the owner's word; nothing else is
pushed, merged into `main`, installed, or published.

The owner settled the story's decisions on 2026-10-12 and asked for this plan.
That is not a build instruction: every phase still gets its own source check,
walkthrough, and explicit instruction to build.

This document is published with the repository. It names no private project,
address, or identifier, and its As built sections will not cite commits from
the private history.

## Outcome

`schedule-send` becomes a public project that a Herdr user can install with one
command on Linux or macOS:

- Homebrew, from the owner's tap, on macOS and on Linux.
- A curl script on Linux, which downloads a checked release binary.
- A Herdr plugin, listed in Herdr's plugin index, that reuses that script.

The repository carries nothing private, keeps its history cleaned, has
numbered releases, runs its checks on every push, and has a README that shows
what the tool does before it explains it.

## Evidence and baseline

Checked read-only against `main` on 2026-10-12:

- `tools/check`: exit 0; all 12 tested Go packages passed, along with vet,
  formatting, shim/shell syntax, and isolated hook checks. About 15 seconds.
- Full `tools/parity`: exit 0; 41 identical cases, 37 documented differences,
  and zero unexpected differences. About 70 seconds.
- These are baseline results, not verification of any phase.
- Tests and parity use temporary state and stand-in commands. No real panes
  were targeted and no real timers were armed. No job was armed on the
  development machine when checked, so nothing pending depends on old paths.

| Finding | Evidence | Work it requires |
| --- | --- | --- |
| The test fixtures are captures of real panes | `testdata/fixtures`: four screen files, two listings, one job; the same names and paths recur in six Go test files and `tools/parity`. They hold private project names, conversation text, costs, two agent session identifiers, and three API request identifiers | Replace them with invented data of the same shape |
| No credentials are in the history | Pattern scan of all 59 commits for API keys, tokens, and private keys: zero hits | None beyond the fixture work |
| The history itself is private | 58 of 59 commits carry a personal address the owner's public repositories do not use, and the captures above are in the history | Publish from a fresh history |
| Homebrew fits the installation design unchanged | `internal/installpath` arms only from the path compiled into the binary, checks the final path component, and resolves symlinked parents. Homebrew's `opt` path is a symlinked parent that survives upgrades | Build from source in the formula with that path. Read from the code; not yet run under Homebrew |
| A downloaded binary cannot carry the user's path | Same package: the recorded path must be absolute and is fixed at build time | Let a release binary record a path relative to the user's home |
| One call stops the macOS build | `internal/term` uses a Linux-only terminal constant. With a two-file shim, a macOS cross-build of the whole program passes vet and links (5.0 MB, measured in a scratch copy) | A per-platform constant |
| systemd is reached from four files | 16 call sites: `internal/cli/cli.go` (5), `internal/cli/status.go` (2), `internal/worker/worker.go` (3), `internal/job/job.go` (6), plus unit rendering and the lifecycle in `internal/job`. Seven test files, 3,143 lines, touch units or `systemctl` | One seam for timer operations before any macOS code |
| Times are parsed by GNU `date` | `internal/when` runs `date -d` | On macOS use `gdate` from Homebrew's `coreutils` (9.12, macOS bottles exist) |
| Release binaries are small and static | `CGO_ENABLED=0` builds: 3.6 MB for x86-64 and 3.3 MB for ARM64, measured | Publish both for Linux |
| Release plumbing does not exist | No `--version`, no version tags (only `zsh-baseline`), no licence, no workflows; the module path is the bare word `schedule-send` | Build all of it |
| The tap already bumps itself | `dj-skn/homebrew-tap` is public and holds one formula, bumped by the source project's release workflow through `mislav/bump-homebrew-formula-action` and a `HOMEBREW_TAP_TOKEN` secret | Follow the same convention |
| Herdr runs on both systems | `herdrdev/herdr`, Apache-2.0, homepage herdr.dev; Homebrew core ships 0.9.3 with Linux and macOS bottles; it also installs by a curl script. Homebrew ships Go 1.27.2; this project needs 1.27.1 | Link both; do not make Herdr a formula dependency |
| Herdr has a plugin index | A plugin is a directory with `herdr-plugin.toml` (required: `id`, `name`, `version`, `min_herdr_version`) and commands Herdr launches. Build commands run at install; Herdr does not install toolchains. There is no plugin update command. The index lists public repositories with the `herdr-plugin` topic and a parseable manifest, refreshed every 30 minutes; 1,684 repositories carry the topic | A manifest whose install step downloads a binary |
| The name is free | No Homebrew formula or cask is called `schedule-send`; `dj-skn/schedule-send` does not exist | None |
| Recording tools are not installed locally | Docker 29.1.3 is present; VHS, a secret scanner, and Homebrew are not. VHS publishes a Docker image, not yet pulled | Run the recorder and Homebrew in containers |

## Decisions settled 2026-10-12

1. **History.** Publish from a new repository, `dj-skn/schedule-send`, that
   starts from one commit. The existing private repository keeps its name and
   its full history as the archive; it is not renamed or deleted. The new
   repository starts private and turns public in P8. The local main checkout
   stays at its path and switches to the new repository. *Revised on
   2026-10-23 in P8: the new repository carries the full history, cleaned.*
2. **Name.** `schedule-send` everywhere a user sees a name: repository, formula,
   command, plugin, and the module path `github.com/dj-skn/schedule-send`.
   Timer names and the state directory keep `herdr-send`, as the compatibility
   rule requires.
3. **Licence.** MIT.
4. **Versions.** Semantic versions tagged `vX.Y.Z`, starting at `v0.1.0`.
5. **Earlier plans.** The SS-1 and SS-2 plan documents are not published.
   *Revised on 2026-10-23 in P8: they stay in the cleaned history, and the
   tree still leaves them out.*
6. **macOS before publishing.** macOS support is part of this story. Mac users
   install with Homebrew; Linux users with the curl script. The same formula
   also serves Homebrew on Linux.
7. **Two tests on real systems.** The rule against arming real timers stands,
   with two exceptions scoped to this story: one launchd job aimed at the
   stand-in Herdr, on the owner's Mac and on GitHub's macOS runners; and
   linking the plugin into the owner's live Herdr to try its popup. No real
   pane is ever a delivery target.
8. **Install path relative to home.** The compiled-in path may be absolute (the
   installer and Homebrew) or relative to the user's home (release binaries).
   After expansion it passes the same checks, and arming from anywhere else is
   still refused. This widens the SS-2 rule; it does not remove it.
9. **Plugin.** A thin wrapper built in this story: a manifest plus the curl
   script, not a second implementation.

## Assumptions

Stated in the walkthrough; the owner did not object.

- On a Mac, the plugin expects the Homebrew install and says so when it is
  missing.
- Fixtures become invented screens of the same shape, with stand-in names of
  the same length, so column-aligned expectations do not move.
- The old zsh script becomes a tracked file so parity runs without the
  `zsh-baseline` tag. Parity stays Linux-only. The zsh shim stays.
- The formula does not pull in Herdr; it tells the user to install it.
- Docker is available wherever the README recordings are regenerated.

## Phases

| Phase | Outcome | Branch | Status |
| --- | --- | --- | --- |
| P1 | A tree that is safe to publish, under its final name | `ss-3-p1` | complete in ss-3 |
| P2 | Timer operations behind one seam, behaviour unchanged | `ss-3-p2` | complete in ss-3 |
| P3 | A launchd backend for macOS | `ss-3-p3` | complete in ss-3 |
| P4 | Versions, release binaries, the curl script, the formula | `ss-3-p4` | complete in ss-3 |
| P5 | The Herdr plugin | `ss-3-p5` | complete in ss-3 |
| P6 | CI and release automation | `ss-3-p6` | complete in ss-3 |
| P7 | README and visuals | `ss-3-p7` | complete in ss-3 |
| P7.1 | The overlay's prompt, and the plugin screenshot | `ss-3-p7.1` | complete in ss-3 |
| P8 | Publication | `ss-3-p8` | in progress |

Each phase starts with a fresh source check and an owner walkthrough. Build
only the approved phase on its phase branch. Every implementation step has a
conventional commit with a body explaining why, after the required gate.
Record As built and verification results, then fast-forward the phase into
`ss-3`. Stop if a fast-forward cannot be made. A main merge, a push, and every
step that changes something on GitHub require separate owner instructions.

The outlines below are the starting point for each walkthrough, not approved
designs. Each phase's section is rewritten with its code check, settled
decisions, and steps when the owner instructs its build.

### P1 — Publishable tree

Swap every real capture and private name for invented ones without changing
what any test proves, then add the files a public repository needs. Behaviour
does not change, apart from two example lines in the usage text.

#### Code check — 2026-10-12

- The usage text printed by `--help` names one of the owner's private projects
  in two example lines, and the old zsh script has the same two lines. A scan
  against the owner's list of private repository names found it after a first
  scan of hand-picked terms had missed it. That list is part of the check.
- The tests read four things from each captured screen: the limit notice, the
  usage figures in the footer, the prompt box, and the rule lines. Everything
  above those is conversation.
- Private names are in 13 files, not only the fixtures: six Go test files and
  `tools/parity` repeat them, one project name on 54 lines, much of it
  column-aligned expected output.
- The owner's timezone is on 44 lines in 12 files, including comments in
  shipped code and the README. 35 pinned times in 7 files depend on it.
- The module rename touches 43 import lines in 19 files and one build flag.
- The zsh baseline is 1,378 lines (51 KB). `tools/parity` reads it from the
  `zsh-baseline` tag, which a fresh history will not have.
- The owner's other public project carries a MIT licence in their full name and
  short contributing, security, and code-of-conduct files.

#### Decisions settled 2026-10-12

The owner agreed to the P1 walkthrough and instructed build.

1. **Timezone.** `Europe/Warsaw` stays. It shows a country and nothing more;
   replacing it would mean recomputing the pinned times and re-proving the
   daylight-saving tests.
2. **Licence name.** The copyright line reads "Jan Halczuk", as on the owner's
   other public project.

Assumptions the owner did not object to: the home directory in tests becomes
`/home/dev`; usage figures and notice wording in the screens stay exactly as
captured while costs, titles, identifiers, and conversation are invented; the
community files match the owner's other public project, kept short, plus a
changelog; the guidance says "keep the baseline file" where it said "keep the
`zsh-baseline` tag"; the README loses only its personal path, since P7
rewrites it.

#### Steps

1. Build the map from each private name to an invented one. It stays outside
   the repository.
   → verify: every stand-in is as long as the name it replaces.
2. Rewrite the four screens, the two listings, and the job fixture, and apply
   the map to the tests and `tools/parity`.
   → verify: `tools/check` passes; parity gives 41 identical, 37 known, 0
   unexpected.
3. Replace the two usage examples. Store the zsh script as
   `testdata/baseline/schedule-send.zsh` with the same two lines changed, and
   point parity at the file.
   → verify: parity gives the same counts from the file as from the tag.
4. Rename the module to `github.com/dj-skn/schedule-send`.
   → verify: `tools/check` and parity, which rebuild through `tools/build`.
5. Remove the SS-1 and SS-2 plans. Take personal paths out of `AGENTS.md` and
   the README, and describe the baseline as a file.
   → verify: both read correctly from top to bottom.
6. Add `LICENSE`, `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`, and
   `CODE_OF_CONDUCT.md`.
   → verify: read each; links that can resolve today do.
7. Scan the tree for the private terms, the owner's private repository names,
   and secret patterns. Run the full gate. Record As built.
   → verify: zero hits; counts quoted.

Not included: the README rewrite (P7), `--version` (P4), the git history and
anything on GitHub (P8).

#### As built — 2026-10-12

- The four screens are invented conversation around the lines the detectors
  read, which are unchanged: the limit notices, the usage figures, the prompt
  boxes, and the rules. Each file keeps its line count. The two listings and
  the job fixture carry invented session and terminal identifiers, the same in
  both listings for the same pane.
- Ten private names map to stand-ins of the same length across seven Go test
  files and `tools/parity`, so no column-aligned expectation moved. The home
  directory in tests is `/home/dev`. The map and the list of private terms are
  kept outside the repository.
- The usage text and the zsh baseline name a neutral project in their two
  example lines. Those two lines are the only difference between
  `testdata/baseline/schedule-send.zsh` and the script at the `zsh-baseline`
  tag. `tools/parity` reads the file; `PARITY_REF` still selects a revision.
- The module is `github.com/dj-skn/schedule-send`, in 43 import lines, the
  linker flag in `tools/build`, and the test that pins that flag.
- `AGENTS.md` and the README name no personal path and describe the baseline
  as a file. The SS-1 and SS-2 plans are removed from the tree.
- `LICENSE` (MIT), `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`, and
  `CODE_OF_CONDUCT.md` exist. The contributing note leaves releasing out
  until P6 builds that process.

Measured on the final tree:

- `tools/check`: exit 0; all 12 tested packages passed, 383 tests and subtests,
  the same number as on `main`.
- `tools/parity`: 41 identical, 37 known differences, 0 unexpected, from the
  file and, at step 3, from the tag as well.
- Private terms: 68 terms, including the owner's private repository names;
  zero hits in the tracked tree, this document included.
- Secrets: 11 patterns with zero real hits (one match is a variable named
  `gaugeToken`), and gitleaks in a container found no leaks in 448 KB of
  tracked files. The only address in the tree is the hook tests'
  `hooks@example.invalid`.

Found while building:

- **A test depended on the developer's machine.** The installation build test
  runs `tools/build` after the harness points `HOME` at a fixed fake path, and
  Go looked for its build cache there. It had passed only because the fake
  path was the developer's real home directory. The test now pins Go's caches
  first. It was confirmed to pass under an unrelated `HOME`. This would have
  failed in CI.
- **The security note depends on a repository setting.** `SECURITY.md` and the
  code of conduct link to GitHub's private vulnerability reporting, which is
  off by default. P8 must turn it on.
- A session title and a folder name that refer to this project itself were
  left in the fixtures; they are not private.

Still to confirm by hand: the three links to `github.com/dj-skn/schedule-send`
cannot resolve until P8 creates the repository.

For later phases: CI (P6) needs neither a particular home directory nor the
`zsh-baseline` tag. P4 and later add their user-visible changes under
Unreleased in the changelog. P6 adds a Releasing section to the contributing
note. P7 still has to move the main-checkout install out of the README's
first screen. P3 has to update the guidance where it says Linux.

### P2 — Timer seam

A move, not a rewrite. Everything that knows about systemd goes into one
Linux-only file, and the rest of the program asks it six things. No output,
message, or file on disk changes.

#### Code check — 2026-10-12

- Six operations cover every use of systemd outside the job package: whether
  jobs can run here, what arming would create, arming, whether a job is still
  armed (`--list`), inspection (`--status`), and removal (cancel, rollback,
  cleanup after firing, and sweeping orphans). The 16 call sites reduce to
  these.
- About 400 lines move and about 60 are new, estimated from reading: 260 in
  `internal/job`, 110 in status inspection, 25 in arming.
- Tests reach systemd through stand-in `systemctl` and `journalctl` commands
  on the path, not through Go code. The names they call directly are few:
  the unit renderers, the unit paths, and the lifecycle methods. If those
  names stay, no test file has to change.
- Error text moves with the code and must stay identical, because parity
  compares error output byte for byte.
- A Linux build passes whether or not systemd detail leaks outside the Linux
  file, so it cannot show the seam is complete. A stub for another platform
  can.
- Some systemd wording is output rather than code: receipts print
  `unit=<name>.timer` and `unit_files=`, and status JSON names its sources
  `timer`, `unit_state`, and `journal`.

#### Decisions settled 2026-10-12

The owner agreed to the P2 walkthrough and instructed build.

1. **No interface.** Each platform has its own file with the same method
   names, and the build picks one. Nothing chooses at run time.
2. **Where it lives.** A Linux-only file in `internal/job`, because removal
   and rollback interleave with the job's lock and state files.
3. **Linux names stay.** The names the tests call keep existing, so no test
   file changes and the existing suite is the proof.

Assumptions the owner did not object to: output wording stays, and P3 decides
what macOS prints; the state key `unit` and the unit-folder setting keep their
names; the `/usr/bin/env` check moves behind the seam with its message
unchanged.

#### Steps

1. Split `internal/job` into a portable file (state, lock, files, command
   runner) and a Linux file (units, `systemctl`, lifecycle), with no logic
   change.
   → verify: `tools/check` passes; no test file changed.
2. Add the neutral scheduler type with "can run", "describe", and "arm", and
   switch scheduling to them.
   → verify: `tools/check`, including the arming and rollback tests; full
   parity.
3. Move "is it armed" and status inspection behind the seam.
   → verify: the status tests pass; full parity.
4. In a scratch copy, add a stub scheduler for macOS and cross-build the
   program.
   → verify: it builds. Nothing from the scratch copy is committed.
5. Run the full gate and record As built.
   → verify: 383 tests; parity 41 identical, 37 known, 0 unexpected; the diff
   touches no test file.

Not included: any macOS code, the per-platform terminal constant, making the
tests run on a Mac, and the output wording. All of those are P3.

#### As built — 2026-10-12

- `internal/job/job.go` holds what every platform needs: state files, the
  lock, atomic writes, the command runner, and the shapes `--status` reports
  (`Diagnostic`, `Inspection`). `internal/job/systemd_linux.go` holds
  everything that names systemd.
- The split itself was a pure move. Compared line by line, the only
  differences were five comments, the import blocks, and the default unit
  directory, which the portable file now asks the platform file for.
- A platform supplies a `Scheduler` and a `Trigger` type and eleven
  functions: `Available`, `Files`, `Render`, `Arm`, `Armed`, `Inspect`, the
  four lifecycle methods `Remove`, `Rollback`, `Cleanup`, `Sweep`, and
  `defaultUnitDir`. On Linux `Scheduler` is another name for the existing
  `Systemctl`, so every name the tests call still exists.
- The CLI and the worker hold a `job.Scheduler`. `internal/cli` no longer
  runs a command or names a unit: its `os/exec` import is gone, and status
  inspection shrank from 212 lines to 105.
- Messages are produced in the Linux file with their wording unchanged. `Arm`
  reports whether enabling was attempted, as the inline code did, and the
  service is still rendered before the job is reserved.

Measured on the final tree:

- `tools/check`: exit 0; all 12 tested packages passed, 383 tests and subtests.
- `tools/parity`: 41 identical, 37 known differences, 0 unexpected, after
  steps 2 and 3.
- The phase's diff touches no test file.
- Seam check, in a scratch copy that was not committed: with a stub of those
  eleven functions and P3's two-file terminal constant, the program
  cross-builds for macOS on ARM64. Without the stub, the first thing missing
  is `defaultUnitDir`.

Differs from the walkthrough: it described six operations. Counted as
functions a platform must provide there are eleven, because removal is four
functions and arming is two.

Still systemd-worded outside the Linux file, by decision: the receipt and
status wording in `internal/show`, the state key `unit`, the `UnitDir` field,
and the test files, which run only against the `systemctl` stand-in.

For P3: the launchd file implements the same eleven functions. The tests need
a launchd stand-in or a Linux-only marking before they can be vetted for
macOS. `Available`, `Arm`, and `Inspect` return user-facing wording, so P3
writes its own messages rather than translating these.

### P3 — macOS backend

A launchd file that provides the same functions as the systemd one, plus the
small portable changes a Mac build needs. None of it can be run on the Linux
development machine, so how it is tested is part of the design.

#### Code check — 2026-10-12

- Four launchd behaviours could not be confirmed from documentation: whether
  a job can unload itself, whether macOS shows a notice for each job file, how
  late a calendar job starts, and what environment a job gets.
- launchd's calendar trigger has no seconds and no year. The worker has to
  wait out the seconds, a job due within the current minute needs a direct
  start, and a skipped job would fire a year later unless guarded.
- Six test files compile only on Linux: all the tests for scheduling,
  delivery, and the job lifecycle. Tests in nine other packages are portable.
- A cross-build shows that Mac-only code compiles and nothing more.
- `tools/build` passes a GNU-only flag to `base64`, so the project cannot be
  built on a Mac with its own script.
- Receipts add `.timer` to the unit name in three places in `internal/show`.
- The worker never parses a time, so GNU `date` is needed only while
  scheduling, where the user's shell provides it.

#### Decisions settled 2026-10-12

The owner agreed to the P3 walkthrough and instructed build.

1. **Test on GitHub's macOS runners while building.** The phase branch is
   pushed to the private repository with a small macOS workflow, which runs
   the Mac tests and a real launchd job. This is the owner's word to push
   that branch; it uses Actions minutes.
2. **Missed jobs.** The Linux rule: asleep at the time means sent on wake;
   switched off or logged out means skipped unless `--persistent`. In
   addition, a job more than 7 days late is never sent.
3. **Job files.** `~/Library/LaunchAgents`, the only place that survives a
   restart. If the run on the owner's Mac shows a notice for every job, the
   alternative comes back to the owner before the phase is merged.

Assumptions the owner did not object to: output keeps the keys `unit` and
`unit_files`, whose values on a Mac name the launchd job and its file; the
systemd test files are marked Linux-only and the launchd file gets its own
tests against a stand-in `launchctl`; `--list` says `active` or `gone` on both
systems; each job logs to a file beside its state and `--status` shows its
last 40 lines; README and `AGENTS.md` change only where they say Linux only.

#### Steps

1. Portable groundwork: the terminal constant, `gdate` on macOS, a portable
   `tools/build`, the display name, and Linux-only marks.
   → verify: `tools/check` passes; parity gives 41 identical, 37 known, 0
   unexpected.
2. A probe workflow on a macOS runner answers the four unknowns.
   → verify: the answers are recorded here before any design depends on them.
3. The launchd file: render, arm, armed, inspect, remove, rollback, cleanup,
   sweep.
   → verify: its tests pass on the runner; a macOS cross-vet joins
   `tools/check`.
4. Worker timing: wait out the seconds and apply the missed-job rule.
   → verify: tests on both systems; parity unchanged.
5. A real launchd job aimed at the stand-in Herdr, on the runner.
   → verify: it fires within the minute, delivers once, and leaves nothing
   behind.
6. The same script on the owner's Mac.
   → verify: the owner sees it pass and reports whether a notice appeared.
7. Full gate and As built.
   → verify: counts quoted for both systems.

#### Probe results — 2026-10-12

Measured by a throwaway workflow on a GitHub macOS runner (macOS 26.6.2, ARM64,
a logged-in `gui` session). The probe branch is not merged.

| Question | Answer |
| --- | --- |
| Can a job unload itself? | Only as its last act. `launchctl bootout` of its own label killed the job at once: nothing after that line ran. Doing the work first, removing the job file, then running bootout left nothing loaded |
| What if a job removes its file and just exits? | It stays loaded until something else boots it out |
| How late does a calendar job start? | 5 seconds after the minute in one measurement |
| Is a calendar event queued while the job is running? | No. A job still running at its calendar minute was not started again afterwards |
| Does a direct start cancel the calendar? | No. A job started with `kickstart` that had exited ran again at its calendar minute |
| What environment does a job get? | `PATH=/usr/bin:/bin:/usr/sbin:/sbin`, with `HOME`, `USER`, and `TMPDIR` set. Homebrew's directory is not on the path |
| Can a job file live outside `~/Library/LaunchAgents`? | Yes, `bootstrap` accepts any path |
| Does macOS record a background item per job file? | For a file in `~/Library/LaunchAgents`, yes: one "legacy agent" record each. For a file bootstrapped from elsewhere, none. Whether a record becomes a visible notice could not be seen on a runner |
| What do failures look like? | `print` and `kickstart` on a missing job exit 113; `bootout` on a missing job exits 3; `bootstrap` exits 5 for a duplicate, a missing file, and a malformed file alike |

What the design takes from this: cleanup removes the job's files and state
first and boots itself out last; the job file carries the scheduling shell's
`PATH`; a job due within two minutes is started at load, with no calendar
trigger; exit 3 and 113 mean "not loaded", and exit 5 cannot tell a duplicate
from a bad file. The background-item finding is evidence for the alternative
in decision 3 and goes back to the owner with the result of the run on their
Mac.

The phase closes only after step 6. Not included: the Homebrew formula (P4),
the permanent CI workflow (P6), and trying a real pane, which stays the
owner's to do by hand.

#### As built — 2026-10-12, steps 1 to 5

- `internal/job/launchd_darwin.go` is the macOS scheduler: one agent per job
  in `~/Library/LaunchAgents`, loaded into the user's `gui` session. A
  platform now provides thirteen functions; the two added in this phase are
  `Name`, which a receipt shows, and `Await`, which a started worker asks.
  `Inspect` also takes the job's paths, since the macOS log is a file.
- A finished job removes its agent, message, and state, then boots itself
  out as its last act. Cancelling and rolling back unload first and keep the
  state when unloading fails. Sweeping unloads agents left without state and
  drops logs a week after their jobs are gone.
- A job due within two minutes has no calendar and starts when loaded. A
  persistent job has both, so a login after its time starts it. The agent
  carries the scheduling shell's `PATH` and XDG roots and says how it was
  armed.
- `internal/job/due.go` holds the timing rule, portable and tested on both
  systems: wait in steps of at most 30 seconds, stand down more than two
  hours early, never deliver more than 7 days late, and drop a non-persistent
  job that was started at load more than five minutes late. A dropped job is
  reported, notified through Herdr, and removed. On Linux the worker is told
  to deliver without the clock being read.
- Portable groundwork: per-system files for the terminal request and the
  date command; `tools/build` no longer needs GNU `base64`; `internal/show`
  prints the name it is given; the tests that drive the `systemctl` stand-in
  are marked Linux-only, and the job package's systemd tests have their own
  file.
- `tools/check` vets for Linux and macOS. `tools/launchd-smoke` arms real
  agents aimed at the stand-in Herdr. `testdata/standin/launchctl` remembers
  what was loaded, so it answers 3 and 113 as launchd does.
- The README and `AGENTS.md` describe both systems. The rule against arming
  real timers now names the smoke test as its one exception.

Measured:

- Linux: `tools/check` exit 0; 397 tests and subtests in 12 packages, 14 more
  than before; parity 41 identical, 37 known differences, 0 unexpected.
- macOS 26.6.2 on an ARM64 runner: `go vet` clean; tests pass in the 10
  packages that have tests there. `internal/cli` and `internal/worker` have
  none on macOS, by the decision to mark them Linux-only.
- Real launchd on that runner, through `tools/launchd-smoke`: the job
  started at load and the calendar job were each delivered exactly once, 0
  seconds after their time; the third was listed as active, inspected, and
  cancelled; no state, message, agent file, or loaded job was left; and
  `--status` of a finished job showed its log.

Found while building:

- **The terminal check was right and its test was not.** A probe on the
  runner showed a real terminal answers the request on macOS, while a pty's
  master end, which the test opened, does not. The test now opens the slave.
- **Two path tests assumed `/home` is a directory.** On macOS it is a
  symlink. Those cases are skipped there and a new test builds its own
  directory and symlink.
- **The smoke test's first run reported success while failing.** It read a
  receipt field that does not exist, and its exit trap hid the error. Both
  are fixed; the passing run above is the corrected script.

Differs from the walkthrough: the smoke test arms three jobs, not one, to
cover both ways a job is started and cancellation. The display name is a
function, and there is a thirteenth, `Await`.

Known limits on macOS, not addressed: a job at a local time that occurs twice
when clocks go back is started at the first and waits for its instant only if
that is within two hours; and a non-persistent job missed while the Mac was
off is loaded again at the next login and listed as `active`, although it
will not fire until the seven-day limit drops it a year later. The second was
removed by the split recorded below.

#### Step 6 — 2026-10-13

The owner ran `tools/launchd-smoke` on their Mac. It ended `launchd-smoke: ok`
with every check passing: the job started at load was delivered once, 2
seconds after its time; the calendar job once, 0 seconds after its time; the
third was listed, inspected, and cancelled; nothing was left on disk or
loaded.

macOS showed two or three background-item notices for the three agent files.
That is the condition in decision 3, so the alternative went back to the owner
before the phase was merged.

#### Decision 3 revised — 2026-10-13

The owner agreed to split job files by kind. An ordinary job keeps its agent
file beside its state and is loaded from there; a `--persistent` job's file
stays in `~/Library/LaunchAgents`, the only place that survives a restart, and
still raises a notice. Never using `~/Library/LaunchAgents` was the
alternative: no notices at all, but `--persistent` would then cover sleep only.

What follows from it:

- An ordinary job is forgotten by launchd at logout or restart, which is the
  rule the owner chose for it. Its state stays and `--list` shows it as
  `gone` until it is cancelled, as a missed job is listed on Linux. The limit
  recorded above, a missed job listed as `active` for a year, no longer
  applies to ordinary jobs.
- A late start can now only mean the Mac was asleep at the time, or the job
  is persistent and was loaded at a later login. Both are delivered, so the
  rule that dropped a job started at load more than five minutes late is
  removed, and the agent no longer tells its worker how it was armed. The
  two-hour and seven-day bounds stay.
- The receipt's file name depends on the flag, so `Files` takes it.
- The smoke test arms a fourth, persistent job and cancels it. One notice is
  expected from a run, for that job.

That no record means no visible notice is inferred from the runner. The
owner's next run confirms or refutes it.

#### As built — the split, 2026-10-13

- An ordinary job's agent is `<state>/<id>.plist`, mode 600; a persistent
  job's is `~/Library/LaunchAgents/herdr-send-<id>.plist`. Removal and
  sweeping look in both places. Where the earlier As built says every agent
  is in `~/Library/LaunchAgents`, and where it describes a five-minute rule
  for jobs started at load, this section replaces it.
- `Files` takes the persistent flag on both systems; systemd ignores it.
- `testdata/standin/launchctl` reads a job's label from its file, as launchd
  does. It had named jobs after their files, which two tests on the runner
  exposed once ordinary jobs had shorter file names.
- `tools/launchd-smoke` arms four jobs. With `LAUNCHD_SMOKE_RECORDS` set, as
  on a runner, it also counts macOS's background-item records.

Measured:

- Linux: `tools/check` exit 0; 394 tests and subtests in 12 packages; parity
  41 identical, 37 known differences, 0 unexpected.
- macOS 26 runner: `go vet` clean; tests pass in 10 packages; the smoke test
  passes, with both timed jobs delivered once, 0 seconds after their time,
  and nothing left behind.
- Background items on that runner: of the four jobs, exactly one has a
  record, the persistent one. The three files beside the state have none.

One commit message on the phase branch is wrong and is corrected by the commit
after it: it says the record check waits a few seconds, and that edit had not
been applied.

Second run on the owner's Mac, 2026-10-13: `launchd-smoke: ok`, every check
passing with four jobs. The job started at load was delivered 2 seconds after
its time and the calendar job 0 seconds after. Ordinary jobs wrote nothing to
`~/Library/LaunchAgents`.

macOS showed one notice during that run, as expected: the persistent job's.
Ordinary jobs raise none. The phase is closed.

For later phases: the formula (P4) needs `coreutils` on macOS for `gdate`, and
a job's agent carries the `PATH` of the shell that scheduled it, so Homebrew's
directory reaches the worker. The permanent workflow (P6) can run
`tools/launchd-smoke` with `LAUNCHD_SMOKE_RECORDS=1` as the throwaway one did.
`internal/cli` and `internal/worker` have no tests on macOS.

### P4 — Versions and installers

Four pieces: a version number, a path rule that lets a downloaded binary arm
jobs, a curl installer for Linux with release archives, and a Homebrew formula.

#### Code check — 2026-10-13

- Homebrew 7.0.9 was measured in a container. A fully qualified
  `brew install <user>/<tap>/<formula>` installed from an untrusted tap in one
  command. Tapping first and installing by bare name was refused until
  `brew trust` is run. The owner's tap README recommends the refused form for
  its existing formula; that was left alone.
- On Linux, Homebrew added gcc and binutils, 598 MB, before building a
  formula from source. The curl script is the right default there.
- The Homebrew image on Docker Hub is a year old (4.6.20) and has to be
  updated inside the container.
- `tools/build` runs its path check with `go run`, which fails when the
  target is another architecture, so it cannot cross-build.
- `internal/installpath`'s test pins the exact build command.
- The message for a build that cannot arm tells the user to run
  `tools/install`, which is wrong for a Homebrew or curl install.
- The formula cannot point at a real download until P8 creates the public
  repository and its first tag.

#### Decisions settled 2026-10-13

The owner agreed to the P4 walkthrough and instructed build.

1. **Install location.** The curl script installs the real file at
   `~/.local/bin/schedule-send`, where Herdr's installer puts Herdr, and a
   release binary arms only from there. No custom location.
2. **Formula.** It lives in this repository until P8 copies it to the tap.
3. **macOS runner.** As in P3: the phase branch and a throwaway branch are
   pushed to the private repository to run the Homebrew install on a macOS
   runner. This is the owner's word to push those branches.

Assumptions the owner did not object to: release archives are Linux only,
x86-64 and ARM64, static, with fixed names and a checksum file; the curl
script checks the SHA-256, refuses on macOS with a pointer to Homebrew, and
upgrades by being run again; `--version` prints `schedule-send <version>`,
with the tag plus commits since it for a checkout build and `dev` without a
tag; the formula builds with the project's build script, needs Go only to
build and `coreutils` on macOS, and tells the user to install Herdr; the
Homebrew image is deleted from the development machine when the phase closes.

#### Steps

1. `--version`, the version stamp and the cross-build fix in `tools/build`,
   and a corrected message for a build that cannot arm.
   → verify: `tools/check` passes; parity counts unchanged.
2. A recorded path may start with `~/`.
   → verify: tests arm from that path under a temporary home and are refused
   anywhere else.
3. `tools/dist` builds both archives and the checksum file.
   → verify: the x86-64 binary runs; both are static; the checksums verify.
4. `install.sh`.
   → verify in a clean Ubuntu container: it installs, reports the right
   version, a dry run shows `~/.local/bin`, a second run upgrades in place,
   and a corrupted archive is refused.
5. The formula and a `tools/brew-smoke` script that installs it from a local
   tap.
   → verify: passes in the updated Homebrew Linux container and on the macOS
   runner, including `brew test` and `brew audit`.
6. The owner runs `tools/brew-smoke` on their Mac.
   → verify: it passes.
7. Full gate and As built.

Not included: putting the formula in the tap, the release workflow (P6), an
uninstall command, and macOS archives.

#### As built — 2026-10-13, steps 1 to 5

- `--version` prints `schedule-send <version>`, or a JSON object with
  `--json`. `tools/build` takes an optional third argument for it, falls back
  to Git's description of the checkout, then to `dev`, and refuses a version
  with characters a linker flag cannot carry.
- `tools/build` builds for another system when `GOOS` and `GOARCH` are set,
  by running its path check for the build machine, and adds
  `SCHEDULE_SEND_LDFLAGS` to the linker flags.
- A recorded path may begin `~/`. It is expanded against the user's home when
  the program runs and then passes the existing checks. `tools/build` accepts
  that form; what follows `~/` must be a clean path that stays under the home.
- `tools/dist VERSION` builds `schedule-send-linux-amd64.tar.gz`,
  `schedule-send-linux-arm64.tar.gz`, and `checksums.txt`: static, stripped
  binaries that record `~/.local/bin/schedule-send`.
- `install.sh` installs the archive for the machine at
  `~/.local/bin/schedule-send`, refusing unless it matches exactly one line of
  the checksum file. `SCHEDULE_SEND_VERSION` picks a release and
  `SCHEDULE_SEND_DOWNLOAD` another source.
- `packaging/homebrew/schedule-send.rb` is the formula. It builds with
  `tools/build`, recording `opt_bin`. Its URL and checksum are placeholders
  until P8.
- `tools/install-smoke` and `tools/brew-smoke` test the two installers from
  the checkout.
- The message for a build that cannot arm names Homebrew and the install
  script as well as `tools/install`.

Measured:

- Linux: `tools/check` exit 0; 396 tests and subtests in 12 packages; parity
  41 identical, 37 known differences, 0 unexpected.
- Archives: 1.5 MB for x86-64 and 1.4 MB for ARM64; both binaries static; two
  builds of the same source gave identical checksums.
- `install.sh`: `tools/install-smoke` passes its 13 checks. The same cases
  passed by hand in a clean Ubuntu 24.04 container, as a new user, with
  archives built beforehand.
- Formula on Linux, Homebrew 7.0.9 in a container: `tools/brew-smoke` passes
  its 7 checks, and `brew style` reports no offences. The run took 70 seconds.
- Formula on macOS 26.6.2 with Homebrew 7.0.9, on a runner: the same 7 checks
  pass, tests pass in 11 packages, and `tools/launchd-smoke` still passes.

Differs from the walkthrough: `brew audit` was not run. It expects a formula
with a real download, which does not exist before P8; `brew style` and
`brew test` were run instead.

Not tested: a job armed by the Homebrew-installed binary firing under real
launchd or systemd. The Homebrew test stops at a dry run, and the launchd
test uses a binary built in a temporary directory.

Step 6, 2026-10-13: the owner ran `tools/brew-smoke` on their Mac and it
ended `brew-smoke: ok`, all 7 checks passing. Homebrew built the formula in
10 seconds, recorded `/opt/homebrew/opt/schedule-send/bin/schedule-send`, and
that path ran the new version after the upgrade. Homebrew upgraded the Mac's
Go from 1.27.1 to 1.27.2 to build it, and asked for confirmation before doing
so. The phase is closed.

For later phases: P5's plugin runs `install.sh` on Linux. P6 can call
`tools/dist`, `tools/install-smoke`, and `tools/brew-smoke` as they are, and
must run `brew update` first, because a runner's Homebrew may predate the
trust rule. P7's install commands must be the fully qualified
`brew install dj-skn/tap/schedule-send`; tapping first and installing by bare
name is refused. P8 fills in the formula's URL and checksum, copies it to the
tap, and should correct the tap README, which recommends the refused form.

### P5 — Herdr plugin

A manifest and a few small shell scripts around the existing program: one
action that opens an overlay pane running `schedule-send` for the focused
agent, and an install step. No Go code changes.

#### Code check — 2026-10-13

- Stable Herdr has no plugin popup. The 0.9.3 client offers overlay, split,
  tab, and zoomed. The documentation that describes a popup is Herdr's "next"
  documentation. The manifest check accepts the word, so a plugin using it
  would fail only when its pane opens.
- On the development machine the Herdr client is 0.9.3 and the running server
  is still 0.9.1. Plugins are run by the server.
- Herdr's own example plugin shows the pattern: an action script asks Herdr to
  open the plugin's pane and hands values over with `--env`. The documentation
  says an action receives the focused pane's id in its environment; that was
  not observed before building.
- `herdr plugin link` under temporary configuration roots validates and
  registers a manifest without touching the live Herdr: it accepted a probe
  manifest with an install step, and refused one that demanded a newer Herdr.
- Invoking an action needs a running server, so the action can only be tried
  in a live Herdr.
- `install.sh` replaces whatever is at `~/.local/bin/schedule-send`, which on
  the development machine is a link to the checkout's build.
- `herdr plugin install` from GitHub needs the public repository, so it cannot
  be tested before P8. The script it runs can.

#### Decisions settled 2026-10-13

The owner agreed to the revised P5 walkthrough and instructed build.

1. **Overlay.** The pane opens as an overlay. No popup path is built.
2. **Minimum Herdr version.** 0.9.3, the current release.
3. **macOS.** The owner tests on their Mac when asked. The walkthrough had
   proposed a macOS runner; the owner's answer was read as preferring their
   own Mac, so only the phase branch is pushed, for that run.

Assumptions the owner did not object to: the plugin is `dj-skn.schedule-send`,
named "Schedule Send", with its manifest at the repository root and its
scripts in `plugin/`; it has one action, and its pane stays open until Enter
is pressed; on Linux the install step runs `install.sh` only when
`schedule-send` is not already installed, and on macOS it requires the
Homebrew install; extra options can be put in a file in the plugin's
configuration folder; for the live check the plugin is linked from the phase
worktree with `--dry-run` in that file and unlinked afterwards; the plugin
binds no key.

#### Steps

1. The manifest and the scripts.
   → verify: Herdr accepts the manifest under temporary roots; `tools/check`
   syntax-checks the scripts.
2. A script test against a stand-in Herdr.
   → verify: the action passes the focused pane; the pane script finds the
   program in each install place and explains when it is missing; the install
   step skips an existing install and, on macOS, demands Homebrew. Passes on
   Linux and on the owner's Mac.
3. A live check in the owner's Herdr, on a 0.9.3 server.
   → verify: the owner invokes the action on an agent pane and sees the prompt
   and a dry-run receipt for that pane in an overlay.
4. Full gate and As built.

Not included: a popup, until stable Herdr has one; a second action to list or
cancel jobs; scheduling automatically when a limit is hit; Windows; and the
plugin index listing, which appears in P8.

#### As built — 2026-10-13, steps 1 and 2 on Linux

- `herdr-plugin.toml` declares `dj-skn.schedule-send`, version 0.1.0, for
  Herdr 0.9.3 on Linux and macOS: one install step, one action (`schedule`),
  and one overlay pane (`prompt`).
- `plugin/schedule.sh` is the action. It asks Herdr to open the pane as an
  overlay and passes the focused pane's id as `SCHEDULE_SEND_TARGET`.
- `plugin/prompt.sh` runs in the pane: it finds the program, adds the options
  from an `args` file in the plugin's configuration directory, runs
  `schedule-send --pane <target>`, and waits for Enter.
- `plugin/setup.sh` is the install step: it leaves an existing install alone,
  runs `install.sh` on Linux when there is none, and on macOS stops with the
  Homebrew command.
- `plugin/find.sh` looks for the program on `PATH`, then in `~/.local/bin` and
  Homebrew's directories, since Herdr starts plugin commands with its server's
  `PATH`.
- `tools/plugin-smoke` tests those scripts against stand-ins and links the
  manifest with the installed Herdr under temporary configuration roots.

Measured on Linux: `tools/check` exit 0; 396 tests and subtests; parity 41
identical, 37 known differences, 0 unexpected; `tools/plugin-smoke` passes its
13 checks, with Herdr 0.9.3 accepting the manifest; a deliberately broken
action made its check fail. The live Herdr was not touched and still lists no
plugins.

Found while building:

- **Some tests fail when the machine is busy.** One gate run failed and the
  next passed. Repeated 12 times in sequence the tests never failed; with four
  suites running at once, 7 of 32 runs failed. The same load on `main` also
  failed 7 of 32, in the same tests: the status-inspection deadline test, and
  tests that call `date` under its two-second limit. They predate this story
  and were left alone. P6 has to deal with them, because CI runners are
  noisier than this machine.
- **The gate's result was being read from its last line.** The commands that
  ran `tools/check` before committing piped it through `tail`, which hides its
  exit status. On the run that failed, the commit recording the Linux steps
  was made and pushed anyway. It changes only this document, and the tree
  passes the gate; from here the exit status itself is checked.

On the owner's Mac, 2026-10-13: `tools/plugin-smoke` ended `plugin-smoke: ok`.
Eleven checks passed, the Linux install was skipped as intended, and Herdr
0.9.3 there accepted the manifest under temporary roots.

Live check on the owner's Mac, 2026-10-13, first part: the live Herdr 0.9.3
linked the plugin from the clone and, on `herdr plugin action invoke`, started
the action. Its reply showed the context Herdr builds for an action:
`focused_pane_id`, `focused_pane_cwd`, `focused_pane_status`,
`invocation_source`, and the tab and workspace ids. The owner then unlinked the
plugin and removed the test binary.

Second part, 2026-10-16: invoked from a shell pane inside the owner's Herdr on
the Mac, the action opened an overlay over that pane, which read "Pane w22:p1
is a plain shell, where the default 'resume' would run as a command. Pass
--message." and then "Press Enter to close." So the overlay opens, the focused
pane reaches the program, the program runs with the options file, and the
pane waits for Enter. The first attempt showed nothing because the action was
invoked from a terminal outside Herdr, where its overlay cannot be seen. The
phase is closed.

Noticed and left alone: on a plain shell pane the plugin can only show that
message, because an overlay offers no way to add `--message`. The action is
named for agents, where it proposes the resume time.

For later phases: the README (P7) should say the action is invoked from
inside Herdr, show the key binding, and note that it is for agent panes. P8
tests `herdr plugin install dj-skn/schedule-send` from GitHub. Not testable before P8: `herdr plugin install` from
GitHub.

### P6 — CI and release

Two workflows that mostly call scripts that already exist: one checks every
push on Linux, Linux ARM, and macOS; one turns a version tag into a GitHub
release and bumps the owner's tap. The release path can only be fully proven
once the public repository exists, so it is rehearsed on the private one.

#### Code check — 2026-10-16

- The busy-machine failures recorded in P5 did not come back: 0 of 32 runs
  with four suites at once, and 0 of 16 with every core also kept busy by 24
  busy loops (measured). On 2026-10-13 the same load failed 7 of 32 on both
  `main` and the branch, so the cause is not settled.
- Most of those failures were in the status-inspection test, which allows
  80 ms and needs a shell to start and print within about 40 ms. The others
  were `date` calls that returned no answer; `date` took at most 29 ms under
  load, so a slow `date` is not the explanation.
- Every check CI needs is a script: `tools/check`, `tools/parity`,
  `tools/install-smoke`, `tools/plugin-smoke`, `tools/brew-smoke`, and
  `tools/launchd-smoke`. Ubuntu runners lack `zsh`, which the gate and parity
  need.
- The version is in three places and nothing checks they agree: the tag, the
  changelog, and the plugin manifest, where Herdr requires one.
- Standard runners are free for public repositories. Private repositories get
  2,000 minutes a month on the Free plan, and macOS costs about ten times Linux
  per minute.
- The tap bump writes to the owner's public tap, so it must never run from the
  private repository.
- `mislav/bump-homebrew-formula-action` computes the checksum of the GitHub
  tag archive by default, the URL form the formula uses. The owner's other
  project uses that action with a `HOMEBREW_TAP_TOKEN` secret.

#### Decisions settled 2026-10-17

The owner agreed to all five recommendations and instructed build.

1. **Flaky tests.** Widen the status test's limits; leave the `date` tests
   until they fail again with a reason. No automatic reruns in CI.
2. **macOS real-system tests** run on every push to `main` and every pull
   request.
3. **Tap bump** commits straight to the tap, then a macOS runner installs from
   the tap as the release's last step.
4. **Release helper.** `tools/release X.Y.Z` updates the changelog and the
   plugin version, commits and tags locally, and prints the push command.
5. **Rehearsal.** The phase branch is pushed to the private repository so CI
   runs there, and a throwaway pre-release tag runs the release workflow with
   the tap bump skipped; that tag and release are deleted afterwards.

Assumptions the owner did not object to: Linux CI installs the current Herdr
with its own script so the manifest is checked against each new release; an
ARM job runs the install test on `ubuntu-24.04-arm`; the release is made with
the preinstalled `gh`, and the only third-party action is the tap bump; after
P8 the tap holds the live formula and `packaging/homebrew` stays as the
template the tests use; Dependabot updates action versions weekly in one
grouped pull request.

#### Steps

1. Widen the status test's limits.
   → verify: 32 runs under load with no failures, and the test still fails
   when the deadline code is removed.
2. `tools/release` and a version-agreement check.
   → verify: in a scratch clone it prepares the commit and tag; the check
   rejects a mismatched manifest or an empty changelog section.
3. `ci.yml` with Linux, ARM, and macOS jobs, and `dependabot.yml`.
   → verify: the Linux steps pass in a clean Ubuntu 24.04 container, then all
   three jobs pass on the private repository.
4. `release.yml`.
   → verify: a throwaway pre-release tag produces a release with both archives
   and the checksums, installs from it on x86-64 and ARM, and skips the tap;
   then the tag and release are deleted.
5. The contributing guide's release section, `AGENTS.md`, and the changelog.
   → verify: read through.
6. Full gate and As built.

Not included: badges (P7), the public repository and the tap token (P8), code
signing, Windows, and coverage reports.

#### As built — 2026-10-17

- `.github/workflows/ci.yml` runs on pushes to `main`, pull requests, by hand,
  and when the release workflow calls it. Linux: the gate, parity, the
  install-script test, and the plugin test, with the current Herdr installed by
  its own script. Linux ARM: the Go tests and the install-script test, which
  runs the ARM binary. macOS: vet, the Go tests, the plugin test, the Homebrew
  formula test, and the real launchd test with the background-item count.
- `.github/workflows/release.yml` runs on a `v*` tag: the version check, the
  CI workflow, `tools/dist`, `gh release create` with the changelog section as
  notes (a pre-release for a version with a hyphen), installs from the release
  on x86-64 and ARM, the tap bump, and an install from the tap on macOS with
  `brew test` and `brew audit --strict --online`. The tap bump runs only from
  `dj-skn/schedule-send` and only for a final version, and fails visibly
  without `HOMEBREW_TAP_TOKEN`.
- `.github/dependabot.yml` groups action updates into one weekly pull request.
- `tools/release VERSION` and `tools/release-check VERSION`, with
  `tools/test-release` in the gate.
- The status deadline test allows 500 ms per inspection instead of 80 ms.
- The contributing guide describes CI and releasing; `AGENTS.md` lists the
  release tools.

Measured:

- `tools/check` exit 0 on Linux; parity 41 identical, 37 known differences,
  0 unexpected; 32 runs of the Go tests, four at once, with no failure.
- The status test: with every fixture command started 100 ms late, the old
  limits failed all four cases and the new ones pass; with the deadlines
  disabled, three cases fail as they should.
- actionlint reports nothing on both workflows, and reported an error seeded
  into a copy.
- In a clean Ubuntu 24.04 container as an ordinary user, the Linux job's steps
  passed: the gate in 22 s, parity in 67 s, and both smoke tests.
- Rehearsal on the private repository, tag `v0.1.0-rc.1` made with
  `tools/release`: the second run passed every job that should run (version 5
  s, Linux 2.5 min, ARM 27 s, macOS 5 min, publish 33 s, installs 7 s each).
  It published a pre-release with both archives and `checksums.txt` and the
  changelog section as notes; `install.sh` installed `0.1.0-rc.1` from it on
  x86-64 and ARM; the tap jobs were skipped. On macOS the Homebrew test and the
  launchd test passed, with one background-item record. Herdr 0.9.5, the
  current release, accepted the manifest on both systems. The release, the tag,
  and the rehearsal branch were then deleted, and the tap's latest commit is
  still from 2026-09-17.

Found while building:

- **The release test would have blocked every release.** It copied the real
  changelog and expected entries under Unreleased, which a release commit has
  just moved. The first rehearsal run failed the gate on exactly that; the
  test now has its own changelog.
- **The plugin test could reach a live Herdr.** This session ran inside a
  Herdr pane, where `HERDR_SOCKET_PATH` sends `herdr` commands to the live
  server even under temporary configuration roots. The live 0.9.1 server
  refused the probe manifest, so nothing was registered; a 0.9.3 server would
  have registered it. `tools/plugin-smoke` now clears every `HERDR_` variable.
  If the owner ran it inside a Herdr pane on their Mac, the plugin may have
  been linked into that Herdr from the clone; the owner's later unlink would
  have removed it.
- **The busy-machine failures did not come back** in any run; their cause is
  still not known. The `date` tests are unchanged, as decided.

Not tested before P8: the tap bump, the install from the tap, `brew audit`,
and `install.sh` downloading anonymously from the public repository.

For later phases: P7's CI badge points at the workflow named `CI`. P8 must put
`Formula/schedule-send.rb` in the tap before the first release, because the
bump action edits an existing formula; add the `HOMEBREW_TAP_TOKEN` secret to
the new repository; then release with `tools/release 0.1.0`.

### P7 — README and visuals

A README written for someone who has never seen the tool, and one script that
records its terminal visuals from the real program inside a container. No
program code changes.

#### Code check — 2026-10-19

- The README is 344 lines and 2,349 words. Its first section is the
  maintainer's own install from a checkout with Git hooks, so a reader meets
  `tools/setup-hooks` before seeing what the tool does.
- The official VHS image (0.12.1, 3.06 GB) is built for x86-64 and ARM, so it
  runs natively on an Apple Silicon Mac. A trial recording of the real program
  against the stand-ins took 18 s and made a 187 KB GIF with the program's
  colours (measured). The image lacks `jq`, which the stand-in Herdr needs; an
  image built on top of it adds it in 5 s.
- The trial program refused to arm because it was not at the path it
  recorded, so the recording script must build it for its place in the
  container.
- At 1200 px the test data's longest target row wraps. The test data also
  reads as test data in a showcase.
- The plugin cannot be recorded in a container: Herdr recognises an agent only
  when a real agent runs in the pane.
- The VHS image includes Chromium, which can render an SVG to PNG so the
  diagrams can be read before they are committed.
- The badges and repository links point at `dj-skn/schedule-send`, which does
  not exist until P8, so they show "not found" until then.

#### Decisions settled 2026-10-19

The owner agreed to all four recommendations and instructed build.

1. **README shape.** A short README, about 150 to 200 lines: what it is, the
   recording, three ways to install, a quick start, the plugin, a
   how-it-works diagram, and links. The full reference moves to
   `docs/usage.md` and the maintainer setup to the contributing guide.
2. **Hero.** The recorded GIF: a limited agent found, the reset proposed, and
   Enter scheduling `resume`. No designed banner.
3. **Theme.** Dark terminal recordings framed as a window, which read on
   GitHub's light and dark pages; the diagram in light and dark variants.
4. **Plugin image.** A screenshot the owner takes in Herdr on an agent pane,
   cropped and placed afterwards.

Assumptions the owner did not object to: a demo scenario of its own in
`testdata/demo`, with the tests' data left alone; media in `docs/media`,
committed with the recording script and its tapes; everything generated and
read here, with the owner running the script once on the Mac; badges for CI,
the latest release, the licence, the Go version, the platforms, and the Herdr
plugin index; Herdr linked to herdr.dev and its repository where first named.

#### Steps

1. A demo scenario in `testdata/demo`.
   → verify: the program shows it correctly through the stand-in; the gate is
   unchanged.
2. `tools/media` and its tapes, recording GIFs and stills into `docs/media`.
   → verify: it runs here, and every still and the hero's key frames are read.
3. The how-it-works diagram as SVG, light and dark.
   → verify: both rendered to PNG and read.
4. The README rewrite, `docs/usage.md`, and the maintainer setup in the
   contributing guide.
   → verify: every relative link and anchor resolves, checked by script; the
   private-term scan is clean; read top to bottom.
5. The owner runs `tools/media` on the Mac and sends the plugin screenshot.
   → verify: the same set comes out.
6. Full gate, As built, fast-forward into `ss-3`.

Not included: a logo, a recording of the plugin, translations, and a
documentation site.

#### As built — 2026-10-19, steps 1 to 4

- `testdata/demo` is the recordings' scenario: a Claude agent stopped at its
  session limit, two Codex agents, and a shell, in invented projects, at
  13:24 UTC on 2026-10-15. `TestDemoScenario` runs the picker against it and
  pins what the hero recording shows.
- The systemctl stand-in's active units are a name or a shell pattern, so the
  recorded job list shows every job as armed.
- `tools/media [TAPE...]` builds the program for `/work/bin` in the container
  and for the Docker machine's architecture, builds `schedule-send-media` on
  VHS 0.12.1 with `jq` added, runs each `docs/media/*.tape` against the demo
  and the stand-ins, and copies the GIFs into `docs/media`. It needs Docker
  and Go.
- `docs/media/resume.gif` is the hero: the picker with the limited agent
  first, the proposal, and the receipt. `docs/media/schedule.gif` schedules a
  message by directory and lists both jobs. Both are dark, framed as a window.
- `docs/media/how-it-works-light.svg` and `-dark.svg`: three steps, in
  GitHub's light and dark colours, chosen with a `<picture>` element.
- The README is 134 lines: badges, one sentence with Herdr linked, the hero,
  the three installs, a quick start with the second GIF, the plugin's key
  binding, the diagram, and links. `docs/usage.md` (280 lines) holds the
  reference with its wording, plus a section on the plugin. The contributing
  guide gained the package map, the checkout install, and how to record the
  media; `AGENTS.md` points at the new places and lists `tools/media`.

Measured:

- `tools/check` exit 0 with 397 tests and subtests; parity 41 identical, 37
  known differences, 0 unexpected; `tools/install-smoke` and
  `tools/plugin-smoke` pass.
- `tools/media` takes about 60 s for both tapes once the image exists. The
  hero is 1500 × 760 px, 12.3 s, 105 KB; the second GIF is 1500 × 460 px,
  14.0 s, 119 KB. Every last frame and a contact sheet of the hero were read.
- `TestDemoScenario` failed when the displayed title or the reset time in the
  demo was changed, and passed again when restored.
- The diagrams were rendered through Chromium with Noto Sans and with Arial
  metrics; every line fits with room. The README was rendered with GitHub's
  stylesheet in light and dark and read top to bottom.
- A script checked 29 relative links and anchors in the Markdown files with
  none broken, and caught three broken links seeded into a probe file.
- The private-term scan found nothing.

Found while building:

- **The first target row wrapped** at 1480 px with the first demo title. The
  title is shorter and the window is 1500 px.
- **The job list showed every job as stopped**, because the stand-in could
  report only one unit as active. Hence the pattern.
- **The list's payload column is 26 characters**, and a longer message pushes
  the job id out of line. That is the program's layout and was left alone;
  the demo uses a short message.
- **The recordings are not byte-for-byte repeatable**: each run gives a
  different file size for the same frames. The contributing guide says to
  commit them only when what they show changes.
- **Herdr's config check does not flag a clash with a default key**, so the
  README's `prefix+alt+s` follows the pattern of Herdr's own example. The
  binding has not been tried in a live Herdr.
- `cmd/schedule-send/main.go` still describes only systemd in its package
  comment; left alone.

Still to do by hand: the owner runs `tools/media` on the Mac, binds the key in
their Herdr, and takes the plugin screenshot, which then goes into the README.

For P8: the CI, release, and Go badges show "repo not found" until the public
repository has run CI on `main` and published a release.

#### As built — 2026-10-22, step 5 on the owner's Mac, and closed

- `tools/media` ran on the owner's Apple Silicon Mac under OrbStack. The
  first run, which downloads and builds the image, took 3 min 54 s in all.
  The same two GIFs came out: the hero's last frame matched the Linux
  recording, and only its length differed (11.8 s against 12.3 s), because
  VHS captures in real time.
- At the owner's request the recordings now use the Claude Dark palette of
  the owner's terminal theme instead of Catppuccin Mocha. The tapes carry the
  16 colours, background, foreground, selection, and cursor as a JSON theme;
  the window frame is the palette's darkest grey; the prompt arrow asks for
  the cyan slot, which that palette shows as amber. The program prints only
  the 16 standard colours, so no program code changed. The hero is now
  94 KB and the second GIF 95 KB.
- `tools/check` exit 0 on the Mac, with 200 tests and subtests passing on
  macOS. The Linux count (397) is from 2026-10-19 and was not re-run; nor was
  parity, which runs on Linux only and covers nothing that changed.
- In the owner's live Herdr 0.9.3, with the plugin linked and `--dry-run` in
  its options file, the README's `prefix+alt+s` binding opened the overlay
  on an agent pane. It is the first time the binding was tried live; Herdr
  reloaded the config with no diagnostics.
- On an agent with no limit notice the overlay showed one line, "no
  usage-limit notice for claude" and the pane, then a bare `at:`. The owner
  judged that too plain for the README, so the screenshot moved to P7.1,
  which reworks that prompt. P7 closes with the README's plugin section in
  text only.

### P7.1 — The overlay's prompt

Inserted on 2026-10-22 after P7's live check. Of four options (clearer text
only; clearer text and a question for the message; a full-screen form; or
nothing), the owner chose the second: the prompt the overlay shows, which is
the program's own terminal prompt, gets clearer text and layout and asks
what to send, so the overlay can send something other than "resume" and
works on a shell pane. The plugin screenshot then goes into the README. A
full-screen form was left out as too large for this story.

#### Code check — 2026-10-22

- The overlay runs `schedule-send --pane <pane>` with the options file, so
  everything it shows is the program's terminal prompt: the usage-limit
  report and the time question in `internal/cli`, drawn by `internal/show`.
- Without `--message`, a run without `--at` sets "resume" before anything is
  asked, and refuses a shell pane outright, since there the text would run
  as a command.
- The prompt reads whole lines from the terminal. Esc cannot cancel; Ctrl-C
  ends the program. Nothing is written to disk before the last answer, so a
  cancel during the questions leaves nothing behind.
- Piped, `--json`, and `--auto` runs print the plain or JSON forms, which
  scripts read; only what goes to the terminal is in scope.
- The Go tests in `internal/cli` and `internal/show` pin the prompt's text,
  including `TestDemoScenario`, which pins the hero recording. Six of the
  seven interactive parity cases run without a message, so their terminal
  output changes; parity runs on Linux only.
- The widest demo picker row is 112 columns, and the hero window holds about
  115 (estimated from the frame).

#### Decisions settled 2026-10-22

The owner chose from previews and instructed build.

1. **Look.** Guided steps: a line down the left (`┌ │ └`) with a marker at
   each question (`◆`), labels and hints dim, nothing new in bold. A quiet
   aligned form and plain sentences were the other two previews.
2. **The message question** is asked wherever the program asks its other
   questions and no message was given, in the overlay and in a terminal
   alike. Enter keeps "resume" on an agent.
3. **Bold.** The picker's `agents` and `panes` headings stay bold.
4. **Parity** runs in a clean Ubuntu container on the owner's Mac, as in P6.
5. **The picker** becomes the first step on the rail. Not answered
   separately: the owner instructed build after it was recommended, which is
   read as accepting it.

Assumptions the owner did not object to: a step keeps its `◆` once answered,
since redrawing printed lines needs cursor control; no terminal-width
detection, so long text wraps; a typed message is one line, with
`--message-file` for more and `--keys` still flag-only; a dry run still ends
with its detail rows, under the rail's end; the story continues on the
owner's Mac.

#### Steps

1. This record; `ss-3-p7.1` from `ss-3`.
   → verify: committed on `ss-3`.
2. The rail's pieces, and the target and limit lines for every limit state,
   used only on a terminal.
   → verify: `internal/show` tests with and without colour; every limit
   state read.
3. The message question: Enter keeps "resume" on an agent; a shell pane needs
   an answer; runs with `--message`, `--keys`, `--at`, `--auto`, or without a
   terminal ask nothing new.
   → verify: `internal/cli` tests for each case, including end of input.
4. The time question and the end: hint, the resolved time, errors, and the
   receipt as the rail's end.
   → verify: `internal/cli` tests; `TestDemoScenario` updated.
5. The picker as the first step.
   → verify: picker tests; the widest demo row measured on the rail.
6. The overlay: Ctrl-C during the questions says nothing was scheduled, and
   the pane still waits for Enter.
   → verify: `tools/plugin-smoke` with an interrupted run.
7. `docs/usage.md` and the README text; both GIFs re-recorded.
   → verify: every last frame read.
8. Parity in an Ubuntu container, with the changed cases documented.
   → verify: no unexpected differences.
9. The owner runs the overlay with `--dry-run` on an agent and on a shell
   pane; the screenshot goes into the README.
   → verify: links and anchors checked by script.
10. Full gate, As built, fast-forward into `ss-3`.

Not included: Esc to cancel, redrawing answered steps, multi-line messages in
the overlay, a full-screen form, and a split pane in place of the overlay.

#### Decision 4 revised — 2026-10-22

During the build the owner asked for the terminal tests to run on the Mac,
where the work is done, and for the full Linux suite to run on their homelab
server once the code is pushed. The Ubuntu container was built and removed
again. Step 8, parity, and the Linux-only Go tests run on the homelab after
the push.

#### As built — 2026-10-22, steps 1 to 7

- `internal/show/rail.go` draws the rail: a top line saying what the run does,
  "dry run" when it is one, and that Ctrl-C cancels; gaps; a `◆` at each
  question in the cyan slot; hints; the answer line; a typed time shown back;
  errors and warnings; and an end for a failure or a cancel. The report gives
  the target's place and every limit state, and a blocked agent, as rail
  lines; the receipt becomes the rail's end, with a dry run's detail rows
  under it. The terminal colours gained cyan.
- `internal/cli` opens the terminal once, at the first question, and keeps it
  to the end, so the picker, the message, and the time read one stream, and
  the rail ends with the receipt, a failure, or a cancel. A warning given once
  the rail is open goes on it. stdout gets the plain or JSON receipt as
  before, and the styled one only when it is not the terminal the rail is on.
- The questions, in order: "Which agent or pane?", the picker with its list
  beside the rail; "What should it send?", asked only when a run is asking
  and no message, file, or keys were given, where Enter keeps "resume" on an
  agent and a shell pane needs an answer; "When?", where Enter keeps the
  proposal and the hint says why, and a typed time is shown back and
  confirmed with Enter, as before. `--auto` still refuses a shell pane.
- Ctrl-C while the run asks ends the rail with "Cancelled. Nothing was
  scheduled." and exit 130; the handler is removed before anything is
  written. `plugin/prompt.sh` traps Ctrl-C, so the pane still waits for
  Enter, and says "Press Enter to close." in line with the rail.
- The cli test harness and the 28 tests that do not depend on systemd moved
  out of the Linux-only `cli_test.go` into untagged files, with the
  launchctl stand-in on PATH so no test reaches launchd. New tests cover the
  rail's pieces, every limit state on the rail, both ends, the message
  question, wrong picker answers, failures, warnings, and Ctrl-C, the last
  with a real SIGINT to the test process.
- The README's quick start and plugin section, `docs/usage.md`, and the
  changelog describe the questions. The hero is re-recorded at 1560 × 960 px,
  13.1 s, 120 KB; the second recording asks nothing and is unchanged.

Measured on the Mac:

- `tools/check` exit 0, with 254 tests and subtests passing on macOS, 50 of
  them in `internal/cli`; before this phase 200 ran there, none of them from
  `internal/cli`'s prompt tests.
- `tools/plugin-smoke` passes, with the new Ctrl-C check under `sh` (bash
  3.2) and `dash`. With the trap removed, the `dash` check failed.
- The built program in a pseudo-terminal, against the stand-ins: Ctrl-C at
  the message question and at the time question each ended with "Cancelled.
  Nothing was scheduled." and exit 130, and nothing was written to its home.
- The widest demo picker row is 115 columns on the rail (112 before) and ends
  near 1462 px; a 1500 px window leaves 1452 px, so the hero grew to 1560 px.
  The hero's middle and last frames were read.

Found while building:

- **None of `internal/cli`'s tests ran on macOS.** Every cli test file was
  Linux-only, so the Mac gate never ran a picker or prompt test. The
  terminal tests now run on both; arming, listing, cancelling, status, and
  lifecycle tests stay Linux-only.
- **The rail adds three columns, not one** as the walkthrough said: the
  picker's rows keep their own indent beside the rail.
- **`dash` dies of Ctrl-C without running its exit trap**, so on Linux the
  overlay closed at once after Ctrl-C; bash, which is `sh` on macOS, ran it.
- **A typed time still needs Enter to confirm it.** The preview showed it
  taken at once; the confirmation was kept.
- **Herdr on the Mac updated itself to 0.9.5** during the session; the
  manifest still asks for 0.9.3.

#### As built — 2026-10-22, steps 8 and 9

- Step 8, on the owner's homelab server, at the pushed `ss-3-p7.1` with the
  steps above, in a worktree so its main checkout stayed on `main`:
  `tools/check` passed; `tools/parity` 41 identical, 37 known differences,
  0 unexpected, the seven interactive cases each naming P7.1;
  `tools/plugin-smoke` 15 of 15, both Ctrl-C checks included, with Herdr
  0.9.3 accepting the manifest; `tools/install-smoke` 13 of 13.
- Step 9, the owner's live check in Herdr on the Mac, with `--dry-run`: the
  overlay opened on an agent with the rail. It showed two faults, both fixed
  on the branch since:
  - The answer was read with the terminal's own echo, so the arrow keys
    arrived as escape codes, showed as `^[[A^[[C`, could not move the
    cursor, and, repeated in the error line, moved it for real and scrambled
    the rail. On a terminal an answer is now read key by key and edited by
    the program: Left, Right, Home, End, Backspace, Delete, and Ctrl-U work,
    other keys do nothing, and Ctrl-C ends the run as cancelled with exit
    130. Anything that is not a terminal is still read a line at a time,
    with escape codes and control characters left out.
  - `+30` was refused; `+30`, `+30m`, and `+2h` now read as offsets, for
    `--at` too.
- After the fixes the owner checked the overlay again and took the
  screenshot, cropped to the overlay and Herdr's bar, without the window
  title, the pointer, or the dry run's paths, and placed in the README as
  `docs/media/plugin.png`, 1226 × 636 px, 64 KB.

Measured on the Mac after the fixes:

- `tools/check` exit 0 with 263 tests and subtests passing; `tools/plugin-smoke`
  passes.
- The built program in a pseudo-terminal: "run the tsts", Left three times,
  "e" gave "run the tests"; Up and Right changed nothing on screen; `+30`
  read as 30 minutes; Ctrl-C while typing ended with "Cancelled. Nothing was
  scheduled." and exit 130. The hero recorded again with the editor came out
  the same, so it was not replaced.
- A script checked the 27 relative links, images, and anchors in the
  Markdown files, none broken, and caught three broken ones seeded into a
  probe file. The private-term list is kept off this machine, so that scan
  was not run; this phase's changes were searched by hand for the owner's
  names, host, and paths, with no hits.

Found while building:

- **The new `+30` test failed once in the gate.** It expected "in 30m", but
  `date(1)` reads the offset a moment after the test notes the time; it now
  expects the delay drawn from the resolved instant, as the `+1 min` test
  does.
- **A relative time keeps its seconds**, so the screenshot shows
  `Thu 22:32:54`. Left as it is: `+30` means thirty minutes from the answer.

#### As built — 2026-10-22, the Linux run of the fixes

The first homelab run of the fixes found two faults and a gap:

- **The `+30` test failed**, as it had once on the Mac, because the fix
  above was not yet pushed.
- **`tools/parity` hung in `confirm-eof`** for about 9 minutes, until the
  run was stopped. The case feeds the program no input. `script` passes that
  on as an end of input while the terminal still reads lines, and Linux
  keeps it as a NUL byte, which it hands over once keys are read one by one.
  The editor ignored the NUL and waited for good. macOS hands over Ctrl-D
  itself, so the Mac never showed it. A NUL on an empty answer now ends the
  run as Ctrl-D does, and a new test sends an end of input to a real
  pseudo-terminal before the answer is read.
- **Parity had no limit on a terminal case**, and a known difference passes
  on any difference, so a hung case could have counted as known. Each
  terminal case is now stopped after 60 s and fails, printed as `HUNG`.

The second run, on the owner's homelab server at the pushed `ss-3-p7.1`
with those fixes: `tools/check` passed; `tools/parity` 41 identical,
37 known differences, 0 unexpected, none hung, in 71 s; `tools/plugin-smoke`
15 of 15; `tools/install-smoke` 13 of 13. The `+30` test passed 50 runs in a
row there, and the terminal tests 10. On the Mac, `tools/check` passes.

Found while building:

- **Ctrl-Space also sends a NUL** in a real terminal, so on an empty answer
  it now ends the run as Ctrl-D does, scheduling nothing; with text typed it
  does nothing. Left as it is: telling it from a kept end of input would
  need the terminal's queue read before the switch.

For P8: the private-term scan was not run on this phase's changes, as the
list is kept off the Mac; it is due before the tree is published.

### P8 — Publication

Publish the repository with its history, cleaned of everything private, and
release `v0.1.0`. The walkthrough on 2026-10-23 changed the story's first
decision: the owner wants the history public, so the work behind the project
can be seen.

#### Code check — 2026-10-23

- The private-term scan, due since P7.1, found nothing at the tip of `ss-3`:
  69 terms, including the owner's private repository names, gave zero hits in
  124 tracked files. Thirteen secret patterns matched only the invented
  request and session identifiers in the test data; gitleaks 8.30.1 in a
  container found no leaks in 669 KB; the only addresses are four at
  `example.invalid`. The plugin screenshot shows a pane name, `~/Downloads`, a
  time, and one-letter workspace labels, with no title bar, pointer, or home
  path, and its metadata holds only the resolution, the size, and the word
  "Screenshot". Both GIFs show demo data only. One private repository created
  after P1 was missing from the list; it had no hits and is now on it.
- `tools/check` passes at the tip of `ss-3` on Linux.
- The history is 146 commits. 145 carry a personal address as author and
  committer; one carries GitHub's noreply address. 87 of its 472 file
  versions hold private names (658 lines), 229 lines hold the developer's home
  directory, and 9 versions of the test screens, listings, and job files are
  the original captures. No commit message holds a private term, and 16 end
  with a link to a private Claude Code session. gitleaks found no leaks in the
  history.
- P1's name map, which includes the home directory, removes every private term
  outside the captured files except three, in old examples in the README, the
  usage text, and the zsh script. The captured files can take P1's invented
  versions, which kept every line the tests read.
- The owner's global Git configuration sets that personal address, so a new
  commit in this checkout would have carried it. The checkout now sets
  GitHub's noreply address itself.
- The tap's bump action edits an existing formula and fails when nothing
  changes. The template points at the `v0.1.0` archive with a zeroed checksum,
  so placed in the tap unchanged, the first release fills in only the checksum.
- GitHub documents private vulnerability reporting for public repositories, so
  it is turned on after the repository goes public.
- A token stored as a secret cannot be read back, so the tap token of the
  owner's other project cannot be reused.
- No timer is armed on the development machine.

#### Decisions settled 2026-10-23

The owner chose after the walkthrough and a talk-through, and instructed build.

1. **History** (revises story decision 1). The full history is published,
   cleaned: every author and committer becomes GitHub's noreply address for
   the owner's account; the name map, extended for the three old examples, is
   applied to every file version; the captured files take P1's invented
   versions; and the session links are removed from the commit messages. The
   new repository is `dj-skn/schedule-send`; the private repository keeps its
   name and its uncleaned history as the archive.
2. **Earlier plans** (revises story decision 5). The SS-1 and SS-2 plans stay
   in the history, cleaned like everything else; the tree still leaves them
   out.
3. **Name.** Story decision 2 stands: `schedule-send` everywhere, the
   repository included, so the plugin installs with
   `herdr plugin install dj-skn/schedule-send`, which takes a repository name.
4. **Tap token.** A fine-grained token that can write only to the tap's
   contents, valid for a year. The owner makes it and adds it as the
   `HOMEBREW_TAP_TOKEN` secret on GitHub's website, so it never passes through
   an agent session.
5. **Outward steps.** The agent runs each with `gh` after the owner's word for
   that step.

Assumptions the owner did not object to: the `zsh-baseline` tag is kept,
cleaned with the rest; the history-rewrite tool runs from a throwaway
environment; after the switch, the old worktrees and branches leave this
checkout, since the archive holds them; the description comes from the
formula, with no homepage, and the topics are `herdr`, `herdr-plugin`, `cli`,
`go`, `systemd`, and `launchd`; the formula goes into the tap just before the
release.

#### Steps

1. This record, on `ss-3`.
   → verify: zero private-term hits in the tree.
2. In a separate clone, merge `ss-3` into `main` and clean the history.
   → verify: the final tree is identical to `ss-3`'s; every file version,
   message, and identity is free of private terms, the home directory, and
   personal addresses; gitleaks finds nothing.
3. An independent read of every commit by four reviewers.
   → verify: every finding fixed or explained, and step 2's checks re-run.
4. Create the private `dj-skn/schedule-send`; push the cleaned `main` and the
   tag.
   → verify: the pushed commits are the checked ones.
5. Switch this checkout: the old repository becomes the `archive` remote and
   the new one `origin`, `main` moves to the cleaned history, the old
   worktrees and branches go, and the program is reinstalled.
   → verify: `main` is clean and tracks `origin/main`; `tools/install` and
   `tools/check` pass.
6. Description and topics; CI runs on the private repository.
   → verify: all three jobs pass.
7. The owner adds the tap token.
   → verify: `gh secret list` shows it.
8. Make the repository public; turn on private vulnerability reporting.
   → verify: the badges and links resolve; the report form opens.
9. The formula into the tap; `tools/release 0.1.0`; push.
   → verify: every release job passes, the tap bump and `brew audit`
   included.
10. Homebrew and the curl script in clean Linux containers, the plugin from
    GitHub, and the index; the owner runs `brew install` on the Mac.
    → verify: each prints `schedule-send 0.1.0`; the index lists the
    repository.
11. As built, on `ss-3-p8` from the published `main`.
    → verify: zero private-term hits; merged on the owner's word.

Steps 4 to 11 each wait for the owner's word. The merge in step 2 reaches
this checkout's `main` only at step 5.

Not included: cleaning the owner's other public repositories, branch
protection, code signing, Homebrew core, and the three items P7.1 left as
they are: Ctrl-Space on an empty answer, the seconds in relative times, and
the manifest's minimum Herdr version.

## Not included

Windows. Prebuilt Homebrew downloads, which would need the recorded path to
survive Homebrew's relocation. A self-update command. Scheduling automatically
when a limit is hit. A time parser that does not need GNU `date`. Migrating
jobs armed before the switch.

## Owner checks and delivery

Only the owner can: run the real launchd test and `brew install` on the Mac;
invoke the plugin in their live Herdr; create the repository, add the tap
token, and change its visibility; approve the main merge, the pushes, and the
tag.

Each phase report states what was checked, what still needs the owner, and
anything that behaves differently from this plan. All other acceptance work
uses the stand-ins in `testdata`, temporary directories, and containers.
