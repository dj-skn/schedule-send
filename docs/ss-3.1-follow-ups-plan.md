# SS-3.1 — What SS-3 left open

## Status

Plan recorded on 2026-11-02 on story branch `ss-3.1`, from `main` after SS-3
closed. The owner settled the decisions below on 2026-11-02 and instructed
the build. P3 and P5 were added at the owner's request after the
walkthrough; each gets its own code check and walkthrough before it is
built.

This document is published with the repository. It names no private
project, address, or identifier.

## Outcome

- The prompt rounds a typed time up to the whole minute and ignores
  Ctrl-Space.
- The project builds with the newest stable Go and asks for a current Herdr,
  and every story starts by moving both to the newest stable release.
- Every release checks the Homebrew install on Linux as well as on macOS.
- The workflows run actions pinned to commits, and Dependabot's updates
  merge themselves once CI passes, as P3's walkthrough settles.
- v0.1.1 is released, its tap written by `tools/bump-tap`.
- The owner's other public repositories carry the noreply address, as P5's
  walkthrough settles.

## Decisions settled 2026-11-02

1. **Minutes.** A typed time that resolves with seconds moves up to the next
   whole minute. A send is then never earlier than asked and at most 59
   seconds later, and every screen shows the time it fires.
2. **Ctrl-Space.** Ignored, as it already is once text is typed. The end of
   input that Linux keeps from before the prompt still ends the run.
3. **Current versions.** At the start of every story, the Go version in
   `go.mod` and the plugin's minimum Herdr move to the newest stable
   releases; this story raises Herdr's to 0.9.5. The rule is in
   `AGENTS.md`.
4. **Go for the builds.** CI, the weekly scan, and the release use
   setup-go's `stable`, so they build with the newest stable Go and no
   version is written down for them. Accepted cost: a new Go release reaches
   CI by itself, and a build is not tied to one Go version by its commit.
5. **Release.** The story ends with v0.1.1.
6. **Homebrew on Linux.** The release's Homebrew install also runs on Ubuntu
   24.04, x86-64 only.
7. **Unfinished branches.** A phase branch is pushed to the public repository
   only when the owner's Mac or CI needs it, after the private-term scan,
   with CI started on it by hand, and deleted after its merge.
8. **Added after the walkthrough.** Pinning actions to commits, auto-merging
   Dependabot's pull requests (P3), and cleaning the commit addresses in the
   owner's other public repositories (P5) are in this story. The walkthrough
   had recommended keeping the last out; the owner chose to include it.

## Phases

| Phase | What | State |
| --- | --- | --- |
| P1 | Housekeeping and current versions | building |
| P2 | The prompt: minutes and Ctrl-Space | planned |
| P3 | Pinned actions and Dependabot's auto-merge | walkthrough first |
| P4 | Release v0.1.1 | planned |
| P5 | The owner's other public repositories | walkthrough first |

P3 comes before the release so that v0.1.1 runs pinned actions. P5 is
outside this repository and can run at any point after its walkthrough.

### P1 — Housekeeping and current versions

#### Code check — 2026-11-02

- `CONTRIBUTING.md`'s row for `docs/media` still mentions the diagram that
  P7.2 deleted.
- `go.mod` names Go 1.27.1, and every workflow reads it through
  `go-version-file`, so CI and releases build with 1.27.1. The newest stable
  is 1.27.3, and Homebrew's Go is 1.27.3. On 2026-10-24 govulncheck listed
  14 known standard-library vulnerabilities in 1.27.1, none reached.
- setup-go v7 resolves `stable` to the newest stable Go in its versions
  manifest.
- `AGENTS.md` and `CONTRIBUTING.md` repeat the Go version; the README
  repeats the Herdr minimum.
- The manifest asks for Herdr 0.9.3. CI checks it with the current Herdr,
  0.9.5, which Homebrew also ships; 0.9.3 was last checked before P7.1.
- The release's Homebrew install runs on macOS only. The Ubuntu 24.04
  runner carries Homebrew, off the PATH. On 2026-11-02 Homebrew on Linux
  installed v0.1.0 from the tap in a container and passed `brew test`.
- Go on the development machine is 1.27.1 with `GOTOOLCHAIN=auto`, so a
  `go` line of 1.27.3 makes it fetch 1.27.3 by itself.

#### Steps

1. The `CONTRIBUTING.md` row.
   → verify: it names only what `docs/media` holds.
2. Go: `go.mod` to 1.27.3, the workflows on `stable`, and `AGENTS.md` and
   `CONTRIBUTING.md` pointing at `go.mod` instead of repeating its number.
   → verify: `tools/check` and `tools/vulncheck` pass under 1.27.3;
   actionlint passes.
3. Herdr 0.9.5 in the manifest and the README.
   → verify: `tools/plugin-smoke` passes, with Herdr 0.9.5 accepting the
   manifest.
4. The release's Homebrew install on macOS and Ubuntu 24.04.
   → verify: actionlint passes; the job itself runs at P4's release.
5. Full gate, As built, fast-forward into `ss-3.1`; push `ss-3.1-p1` and
   start CI on it.
   → verify: all three systems and the scan pass, the log showing go1.27.3.

Not included: the versions other tools are pinned to, such as govulncheck's
in `tools/vulncheck`.

### P2 — The prompt: minutes and Ctrl-Space

#### Code check — 2026-11-02

- `show.Time` in `internal/show/show.go` adds the seconds whenever an
  instant has them. `internal/when` hands a typed time to GNU `date`, which
  resolves `+30` to the second, so a relative time always shows them.
- No `tools/parity` case uses a relative time.
- `internal/term/answer.go` takes a NUL byte for Ctrl-D, both for the end of
  input Linux keeps and for Ctrl-Space. The kept one is already queued when
  the prompt starts reading keys one by one; a typed one cannot be yet.

#### Steps

1. Round a resolved typed time up to the whole minute.
   → verify: tests at :00 and :59 seconds and on the minute; `tools/parity`
   unchanged.
2. Ignore Ctrl-Space; keep a queued end of input ending the run.
   → verify: real-terminal tests on Linux for both; `confirm-eof` passes
   without hanging; `tools/check` on the owner's Mac.
3. Changelog entries, full gate and parity, As built, fast-forward into
   `ss-3.1`.

### P3 — Pinned actions and Dependabot's auto-merge

Added on 2026-11-02 at the owner's request. Its code check and walkthrough
come before it is built. It starts from these questions:

- **Pinning.** Each `uses:` names a full commit, with its version as a
  comment that Dependabot keeps up to date.
- **Auto-merge.** GitHub merges a pull request by itself only once its
  required checks pass, and `main` requires none, since SS-3 left out
  branch protection. Auto-merge needs a rule on `main` that requires CI;
  how that rule meets the owner's direct pushes to `main` has to be
  settled.
- **Which updates** merge themselves, and which wait for the owner.

### P4 — Release v0.1.1

#### Steps

1. `tools/release 0.1.1`; push the commit and the tag on the owner's word.
   → verify: every release job passes; `tools/bump-tap` writes the tap and
   reads it back; Homebrew on macOS and on Ubuntu prints
   `schedule-send 0.1.1`.
2. As built.
   → verify: zero private-term hits.

### P5 — The owner's other public repositories

Added on 2026-11-02 at the owner's request. The commit history of some of
the owner's other public repositories, the Homebrew tap among them, carries
a personal address rather than GitHub's noreply one. This document does not
name the others. Its code check and walkthrough come before it is built.
It starts from these facts:

- Rewriting a published history changes every commit identifier. A clone
  made before keeps the old commits, and GitHub keeps the refs of pull
  requests, so cleaning hides the address from new readers, not from
  earlier copies.
- Everyone who installs from the tap holds a clone of it, so how
  `brew update` takes a rewritten tap is checked before the tap is touched.

## Not included

Code signing, Homebrew core, and Windows, as in SS-3.

## Owner checks and delivery

Only the owner can: run `tools/check` and the prompt on the Mac; approve
pushes of `main`, of the tag, and of a phase branch; and approve P5's
rewrites. Each phase report states what was checked, what still needs the
owner, and anything that behaves differently from this plan.
