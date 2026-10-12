# SS-3 — Open-source release for Linux and macOS

## Status

Plan recorded on 2026-10-12 on story branch `ss-3`, starting from `main`. P1
is built and verified on `ss-3-p1`; this update accompanies its fast-forward
into `ss-3`. P2 to P8 are not started. Nothing is merged into `main`,
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
| P2 | Timer operations behind one seam, behaviour unchanged | `ss-3-p2` | approved 2026-10-12, in progress |
| P3 | A launchd backend for macOS | `ss-3-p3` | not started |
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

### P3 — macOS backend

Add the launchd implementation in a macOS-only file, the per-platform terminal
constant, `gdate`, and a per-job log file that `--status` reads in place of the
journal. Points to settle in the walkthrough, none of them testable from Linux:

- launchd's calendar trigger has no seconds and no year. The worker would wait
  for the stored firing time and the job must remove itself after one run.
- A launchd job is timer and service in one, so the worker cannot disable its
  own trigger the way a systemd service disables its timer.
- launchd runs a missed calendar job on wake. The worker would decide between
  running and skipping from the stored firing time and `--persistent`.
- Job files under `~/Library/LaunchAgents` survive a restart but may raise a
  background-item notice per job; files elsewhere last only for the login
  session.
- launchd starts jobs with a minimal `PATH`, so tool paths must be absolute.

→ verify: Go tests against a launchd stand-in; a macOS cross-build added to the
gate; one real launchd job aimed at the stand-in Herdr on the owner's Mac.

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
