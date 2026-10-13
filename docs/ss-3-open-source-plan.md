# SS-3 — Open-source release for Linux and macOS

## Status

Plan recorded on 2026-10-12 on story branch `ss-3`, starting from `main`. P1
and P2 are complete in `ss-3`. P3 is built on `ss-3-p3` and verified on Linux
and on a macOS runner; it is not merged, because it closes only after the run
on the owner's Mac. P4 to P8 are not started. Nothing is merged into `main`,
installed, published, or pushed.

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

The repository carries nothing private, starts from a fresh history, has
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
   stays at its path and switches to the new repository.
2. **Name.** `schedule-send` everywhere a user sees a name: repository, formula,
   command, plugin, and the module path `github.com/dj-skn/schedule-send`.
   Timer names and the state directory keep `herdr-send`, as the compatibility
   rule requires.
3. **Licence.** MIT.
4. **Versions.** Semantic versions tagged `vX.Y.Z`, starting at `v0.1.0`.
5. **Earlier plans.** The SS-1 and SS-2 plan documents are not published.
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
| P3 | A launchd backend for macOS | `ss-3-p3` | built; awaiting the owner's Mac run |
| P4 | Versions, release binaries, the curl script, the formula | `ss-3-p4` | not started |
| P5 | The Herdr plugin | `ss-3-p5` | not started |
| P6 | CI and release automation | `ss-3-p6` | not started |
| P7 | README and visuals | `ss-3-p7` | not started |
| P8 | Publication | `ss-3-p8` | not started |

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

Still to confirm: a second run on the owner's Mac, expecting one notice
instead of one per job.

### P4 — Versions and installers

Add `--version`, stamped at build time from the tag. Build static Linux
binaries for x86-64 and ARM64 with checksums. Accept a home-relative recorded
path. Write the curl script: detect the architecture, download, verify the
SHA-256, install to the fixed location, link the command, and accept a local
source for testing. Write the formula on a branch of the tap: build from source
with Homebrew's upgrade-proof path, depend on `coreutils` on macOS, and tell
the user to install Herdr.

→ verify: the curl install in a clean Ubuntu container; the formula in a
Homebrew Linux container, where a dry run shows the upgrade-proof path;
`brew install` from the tap branch on the owner's Mac.

### P5 — Herdr plugin

Add `herdr-plugin.toml` with an install step that runs the curl script on Linux
and checks for the Homebrew install on macOS, and one action that opens the
existing confirm prompt in a popup for the focused agent. Unverified so far:
which action contexts exist, the lowest Herdr version that supports popups, and
whether Herdr's plugin registry can be isolated under a temporary home.

→ verify: Herdr accepts the manifest; the owner invokes the action in their
live Herdr.

### P6 — CI and release

A workflow on every push and pull request: the gate and parity on Linux; vet,
tests, and the real launchd job on macOS. A workflow on a `v*` tag: check the
tag against the changelog, build the binaries and checksums, create the GitHub
release, and bump the tap. Keep action versions current with Dependabot.

→ verify: the Linux steps pass in a clean Ubuntu container before anything is
pushed; the workflows themselves can only be proven in P8.

### P7 — README and visuals

Rewrite the README install-first: what it is, a recording, the three install
commands, a quick start, then reference. Add badges for CI, the latest release,
and the licence. Link Herdr's site and repository where Herdr is first named.
Add a hero graphic and a how-it-works diagram as SVG with light and dark
variants. Record the terminal sessions with one script that runs the real
binary against the stand-ins inside a container, so it works on Linux and on a
Mac with Docker.

→ verify: every image is opened and read; the owner runs the script once.

### P8 — Publication

Each outward step waits for the owner's word: merge `ss-3` into `main`; create
the private `dj-skn/schedule-send`; push one commit built from that tree;
switch the local checkout to it; set the description, homepage, and topics
(`herdr` and `herdr-plugin` among them); turn on private vulnerability
reporting, which the security note links to; add the tap token; confirm CI
passes; make the repository public; tag `v0.1.0`.

→ verify: the badges are green; Homebrew installs on a Mac and in a Linux
container; the curl script installs on a clean Linux machine; the plugin
installs from GitHub and appears in Herdr's index.

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
