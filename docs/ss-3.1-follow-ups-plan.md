# SS-3.1 — What SS-3 left open

## Status

Plan recorded on 2026-11-02 on story branch `ss-3.1`, from `main` after SS-3
closed. The owner settled the decisions below on 2026-11-02 and instructed
the build. P3 and P5 were added at the owner's request after the
walkthrough; each gets its own code check and walkthrough before it is
built. P1 is complete in `ss-3.1`, fast-forwarded from `ss-3.1-p1` after CI
passed on it. P2 is built and in `ss-3.1`, fast-forwarded from `ss-3.1-p2`
after CI passed on it; the owner's Mac run is still to come. The owner
settled P3's decisions on 2026-11-03 and instructed its build, and added
P3.1 from a screenshot of the picker. P3 is built and in `ss-3.1`, after CI
passed on it. On 2026-11-03, ahead of P3.1, the owner merged `ss-3.1` into
`main`, pushed it, and applied P3's settings: P4's first step. Dependabot's
first pull request then merged itself. P3.1 and the release reach `main`
through a second merge of `ss-3.1`. The owner settled P3.1's decisions on
2026-11-03 and instructed its build. P3.1 is built and in `ss-3.1`, after CI
passed on it.

This document is published with the repository. It names no private
project, address, or identifier.

## Outcome

- The prompt rounds a typed time up to the whole minute and ignores
  Ctrl-Space.
- The project builds with the newest stable Go and asks for a current Herdr,
  and every story starts by moving both to the newest stable release.
- Every release checks the Homebrew install on Linux as well as on macOS.
- The workflows run actions pinned to commits, and Dependabot's patch and
  minor updates merge themselves once CI passes.
- The picker shows which agents already have a send scheduled.
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
| P1 | Housekeeping and current versions | complete |
| P2 | The prompt: minutes and Ctrl-Space | built; Mac run to come |
| P3 | Pinned actions and Dependabot's auto-merge | complete |
| P3.1 | The picker shows sends already scheduled | built; Mac look to come |
| P4 | Release v0.1.1 | step 1 done; release after P3.1 |
| P5 | The owner's other public repositories | walkthrough first |

P3 comes before the release so that v0.1.1 runs pinned actions, and P3.1 so
that it carries the picker change. P5 is
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

#### As built — 2026-11-02

- `CONTRIBUTING.md`'s row names the recordings, their tapes, and the plugin
  screenshot.
- `go.mod` names Go 1.27.3. CI, the weekly scan, and the release take
  setup-go's `stable`, with a comment that `go.mod` names only the oldest Go
  that builds. `AGENTS.md` and `CONTRIBUTING.md` point at `go.mod` for the
  version, and `AGENTS.md` carries the rule on current versions.
- The manifest and the README ask for Herdr 0.9.5. The changelog's
  Unreleased section records both version changes.
- The release's Homebrew install runs on macOS and on Ubuntu 24.04, and
  neither cancels the other; on Ubuntu a first step puts Homebrew on the
  PATH. `CONTRIBUTING.md` says so.

Measured:

- `tools/check` exit 0, 12 packages, under Go 1.27.3, which Go fetched by
  itself. `tools/parity`: 41 identical, 37 known differences, 0 unexpected.
  `tools/vulncheck`: no vulnerabilities in either build.
  `tools/plugin-smoke`: Herdr 0.9.5 accepts the manifest. actionlint 1.7.7
  passes every workflow.
- The new Linux job's commands, from `brew update` to
  `brew audit --strict --online`, passed for v0.1.0 in a throwaway Homebrew
  container, removed afterwards.
- CI on the pushed `ss-3.1-p1`, started by hand: Linux, Linux ARM, macOS,
  and the scan passed, each with go1.27.3 from `stable`. On macOS,
  `tools/brew-smoke` built the checkout with Homebrew's Go 1.27.3.

Found while building:

- **Homebrew caps the `go` line.** Homebrew builds with `GOTOOLCHAIN=local`,
  so a `go` line newer than its `go` formula would break `brew install`.
  The rule in `AGENTS.md` keeps the line at or below Homebrew's Go; CI's
  macOS `tools/brew-smoke`, which builds the checkout with Homebrew, would
  show a breach on the next push.
- **The README's Herdr version is a second place to move.** Nothing checks
  that it matches the manifest.

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

#### As built — 2026-11-02 to 2026-11-03

- `when.Resolve` moves a typed time without seconds up to the next whole
  minute, with `when.UpToMinute`. Seconds typed on purpose, as in
  `18:30:15` or `@epoch`, are kept, since `--at` documents `HH:MM:SS`. A
  notice's own time is read without the rounding, since the limit logic
  compares it with now as the agent printed it.
- "One minute from now", proposed once a limit has passed, moves up to a
  whole minute the same way, so the prompt shows no seconds there either.
- On a terminal, a NUL ends an empty answer only if it was already waiting
  when the prompt switched to reading keys. The switch counts the waiting
  bytes, with `TIOCINQ` on Linux and `FIONREAD` on macOS, whose value
  package `syscall` leaves out. A NUL typed later, which is Ctrl-Space,
  does nothing. If the count fails, every NUL ends an empty answer, as
  before, so a run fed no input still cannot wait for good.
- The usage reference and the changelog describe both.

Measured:

- `tools/check` exit 0, 12 packages. `tools/parity`: 41 identical, 37 known
  differences, 0 unexpected; `confirm-eof` finished without hanging.
- The terminal tests passed 20 runs in a row. Two planted bugs each fail
  them: every NUL ending the answer fails three tests, and nothing counted
  as waiting fails two, the kept end of input among them, which then
  waits.
- CI on the pushed `ss-3.1-p2`, started by hand, passed on Linux, Linux
  ARM, and macOS, and the scan. On macOS the real-terminal tests passed, so
  the `FIONREAD` value holds there, and `tools/launchd-smoke` passed.

Found while building:

- **The launchd test assumed seconds.** It armed its job due soon at
  `+75 sec`, which now rounds up to as much as 134 seconds, past the two
  minutes within which launchd starts a job when it is loaded. CI's first
  macOS run failed on that check; the test now arms `+40 sec`, 40 to 99
  seconds out. The test runs on macOS only, so the Linux gate could not
  show it.
- **The delay shown counts clock minutes.** `+30` typed at 22:02:54 reads
  "22:33, in 31m": the delay rounds up, as before, and 22:02 to 22:33 is 31
  minutes on the clock.
- **Two sends to one pane within a minute now share a time.** The second
  is refused as already scheduled, as two sends at the same typed time
  always were.

Still to confirm by hand: `tools/check` on the owner's Mac, and Ctrl-Space
and a `+30` answer in the live prompt.

### P3 — Pinned actions and Dependabot's auto-merge

Added on 2026-11-02 at the owner's request.

#### Code check — 2026-11-03

- The workflows name two actions in 15 places: `actions/checkout` (v7.0.1)
  and `actions/setup-go` (v7.0.0). Auto-merge adds
  `dependabot/fetch-metadata` (v3.1.0).
- With `@v7`, each workflow follows every v7 release by itself, so
  Dependabot proposes only a new major; it has opened no pull request. A
  pinned commit turns every release into an update, grouped into one pull
  request a week at most. Dependabot rewrites the version comment beside a
  pinned commit, which its source shows (`version_commenter.rb`).
- `main` has no branch protection and no ruleset, and the repository does
  not allow auto-merge. GitHub offers auto-merge only while a required check
  is pending. A rule that requires CI also refuses a direct push, unless the
  pusher's role is on its bypass list.
- The repository setting "Require actions to be pinned to a full-length
  commit SHA" (`sha_pinning_required`, off) refuses a workflow that uses an
  unpinned action; a reusable workflow may still be named by path or tag.
  Turned on while `main` is unpinned, it would stop CI there and the weekly
  scan.
- A pull request runs CI, not the release's own jobs: the tap write and the
  Homebrew installs run only for a tag.
- `dependabot/fetch-metadata` reports the largest version step in a grouped
  pull request as `update-type`. Dependabot already waits three days after
  a release before proposing it; `cooldown` changes that for Actions with
  `default-days` only, and security updates never wait.
- CI took 7 minutes on the last run, macOS the longest.

#### Decisions settled 2026-11-03

1. **The owner's pushes.** A ruleset on `main` requires the four CI checks,
   Linux, Linux ARM, macOS, and Vulnerabilities, and the repository admin
   role may always bypass it. The owner's direct pushes go through with a
   notice.
2. **What merges itself.** Patch and minor updates, once CI passes. A major
   waits for the owner, since CI does not run the release's own jobs.
3. **Enforcing the pins.** GitHub's setting, and a check in `tools/check`
   that fails before an unpinned line is pushed.
4. **The settings.** The ruleset, auto-merge, and the pin setting are applied
   with `gh` right after `ss-3.1` is pushed to `main` at P4, on the owner's
   word then.

Built on these, which the owner did not object to: squash merges, so an
update is one `chore:` commit on `main`; a seven-day wait after an action's
release; and `actions/checkout` pinned at v7.0.0, one release behind, so
Dependabot's first run proposes v7.0.1 and tests the auto-merge end to end.

#### Steps

1. Pin the 15 references as `@<commit> # vX.Y.Z`.
   → verify: no `uses:` of an action without a 40-character commit; CI
   passes on the pushed phase branch.
2. `tools/check` refuses an unpinned action.
   → verify: a planted `@v7` fails it; the hook tests still pass.
3. The auto-merge workflow: Dependabot's pull requests only; it reads the
   update type and turns on squash auto-merge for patch and minor.
   → verify: actionlint is clean; the job is skipped on our own branch.
4. Dependabot waits seven days for Actions.
   → verify: GitHub reports no configuration error after the P4 push.
5. `AGENTS.md` and `CONTRIBUTING.md`: actions are pinned, and how updates
   merge.
   → verify: `tools/check`; zero private-term hits.
6. As built, fast-forward into `ss-3.1`.

Not included: a merge queue, which one pull request a week does not need,
and the other repositories' workflows, which P5 looks at.

#### As built — 2026-11-03

- The 15 uses of an action name a commit with its version beside it:
  `actions/checkout` at v7.0.0, one release behind on purpose, and
  `actions/setup-go` at v7.0.0. Each commit was read from its tag on
  GitHub. The release's call of `ci.yml` is still by path.
- `tools/check` fails on a `uses:` under `.github` that names neither a
  40-character commit with a `# v` version after it nor a path in this
  repository.
- `.github/workflows/auto-merge.yml` runs on pull requests that Dependabot
  opened in this repository. `dependabot/fetch-metadata` (v3.1.0, pinned)
  reads the update and refuses commits Dependabot did not make; for a patch
  or minor it turns on squash auto-merge with the workflow's own token.
- Dependabot proposes an action's release once it is seven days old.
- `AGENTS.md`, `CONTRIBUTING.md`, and the changelog say so. P4's first step
  now carries the three settings as `gh` commands, checked against GitHub's
  published REST description: the ruleset's fields, role 5 as the
  repository admin, `allow_auto_merge`, and `sha_pinning_required`.

Measured:

- `tools/check` exit 0, 12 packages. The pin check failed each of four
  planted faults: `@v7`, a commit without its version, a short commit, and
  a quoted tag; it passed the pinned tree.
- The Dependabot configuration validates against its published schema,
  `cooldown` included, with no errors.
- CI on the pushed `ss-3.1-p3`, started by hand, passed on Linux, Linux
  ARM, and macOS, and the scan; its log shows both actions downloaded at
  the pinned commits.

Found while building:

- **actionlint did not run.** Auto mode refused to download it, as code
  from outside. The workflows parse as YAML and CI ran the three changed
  ones; the auto-merge workflow runs only on a pull request, so its first
  real run is Dependabot's first pull request after P4.
- **checkout v7.0.1 has fixes.** It escapes values passed to `git config
  --unset` and narrows a check on pull requests. Running v7.0.0 until
  Dependabot's first pull request merges was accepted for the end-to-end
  test; none of the fixes touches how these workflows check out.
- **Settings and the first push.** The ruleset goes on after `main` is
  pushed, so the release push in P4's second step is the first to show the
  bypass notice.

Still to confirm, at P4: the settings take, the release passes under them,
and Dependabot's checkout v7.0.1 pull request merges itself.

### P3.1 — The picker shows sends already scheduled

Added on 2026-11-03 from the owner's screenshot: two agents held by the same
limit, one of them already scheduled, and nothing in the picker to tell
them apart.

#### Code check — 2026-11-03

- The picker reads each agent's screen for its limit, never the scheduled
  sends. Each send's state records its pane, agent, and folder; asking
  whether its timer is armed is one call per send, 10 ms measured with
  `systemctl --user is-active` on Linux, and `launchctl print` on macOS,
  not measured.
- A send is delivered only if its pane still runs the same agent in the
  same folder (`internal/worker`); a pane id alone can belong to another
  agent by then.
- The plugin passes the pane it opened on, so the run skips the picker and
  opens the rail with the target's report in `detectAt`.
- Two sends to one pane at the same minute share an id. The second is
  refused after the message and the time are answered, and the run ends.
  P2's rounding makes accepting the proposed reset twice land on the same
  minute.
- Nothing reads the window's width. A picker row needs 64 columns before
  its title, the rail's top line 58; the screenshot is about 52.
  `TIOCGWINSZ` is in package `syscall` on Linux and macOS.

#### Decisions settled 2026-11-03

1. **The mark.** A row with a send reads "queued" in the status column,
   with its own symbol, and its note adds "sends 22:51", or "2 sends, next
   22:51"; the limit's "resets 22:50" stays.
2. **Order.** Within a group, rows with a send come after those without,
   so Enter takes the one still waiting; `--pick best` follows the same
   order.
3. **A clash at "When?".** The time is checked when it is confirmed; a
   clash says so and asks again, keeping the message. A time given with
   `--at` is refused as before.
4. **Narrow windows.** A row fits the window: the title goes first, then
   the folder shortens from the left; the limit and send notes stay. The
   rail's top line drops its padding.

Built on these, which the owner did not object to: a send counts while its
timer is armed, including one being delivered; a gone or invalid one does
not. Plain panes get the same mark. The report that opens the questions
says "already has a send at 22:51 · this adds another". The list printed
when several targets match shows the marks, and `--json` gets a note.

#### Steps

1. Find each pane's scheduled sends: state that loads, the same agent and
   folder, an armed timer.
   → verify: unit tests for a match, a reused pane, a gone timer, an
   invalid state, and several sends.
2. The mark and the order in the picker, and `--pick best`.
   → verify: the screenshot's case, two limited agents and one scheduled:
   Enter takes the other, and the row reads "queued … sends 22:51".
3. The line in the report when the target is given (the plugin,
   `--pane`), and a note otherwise.
   → verify: unit tests; `tools/plugin-smoke`.
4. The clash check at "When?".
   → verify: a real-terminal test: the scheduled minute asks again and the
   message is kept; `tools/parity duplicate` unchanged.
5. Rows and the top line fit the window.
   → verify: rendering tests at 52, 64, and 100 columns; no width, as
   through a pipe, is unchanged.
6. Usage reference, changelog, full gate and parity, As built,
   fast-forward into `ss-3.1`.
   → verify: `tools/check`; `tools/parity` 0 unexpected; CI on macOS.

Not included: cancelling or replacing a send from the picker; `--cancel`
stays the way.

#### As built — 2026-11-03

- `internal/cli/queued.go` finds, once per run, when each pane's sends
  fire: a state that reads, the same kind, agent, and folder as the pane
  shows now, and a timer the scheduler calls active. It asks the scheduler
  only about sends that match a pane.
- The picker draws such a target as "» queued", its note "sends 16:01" or
  "2 sends, next 16:01" after any limit's reset, and lists it after the
  targets without one in its group; `--pick best` takes the same first row.
  The list printed when several targets match shows the marks.
- The report that opens the questions ends with "» already has a send at
  Sat 16:30 · this adds another". A run that asks nothing gives it as a
  note, a JSON note under `--json`, and after the picker it goes beside the
  rail.
- "When?" checks a typed time and the time Enter keeps against the stored
  sends; a clash says "wA:p1 already has a send at Sat 16:01. Type another
  time." and asks again, keeping the message. `--at` is refused as before.
- `term.Width` reads the terminal's columns with `TIOCGWINSZ`. A picker row
  too wide drops its title, keeping its notes in the title's column while
  they fit; then shortens its folder from the left, at a "/" where one
  fits; then puts its notes on a second line, with the title when it fits
  there. The rail's top line drops its padding, then "Ctrl-C cancels". With
  no width, as through a pipe, the picker is unchanged byte for byte.
- The systemctl stand-in takes several unit patterns, so a test can arm
  some sends and not others; the usage reference and the changelog
  describe the change.

Measured:

- `tools/check` exit 0, 12 packages; `tools/parity`: 41 identical, 37
  known differences, 0 unexpected. The picker, plugin, and clash tests
  passed three runs in a row.
- Planted faults each failed their tests: no folder-and-agent check, no
  armed check, no reordering, no clash check at Enter.
- No line of the drawn picker is wider than its width at any width from 40
  to 120. The built program, in a real terminal against the stand-ins,
  drew its widest line at 52 columns of a 52-column window, and at 98 of
  100.
- CI on the pushed `ss-3.1-p3.1`, started by hand, passed on Linux, Linux
  ARM, and macOS, and the scan; on macOS the width test, `tools/plugin-smoke`,
  and `tools/launchd-smoke` passed.

Found while building:

- **Names from the screenshot reached the public repository.** A test and
  a comment copied the screenshot's home folder, project folder, and pane
  ids. The private-term scan counted 21 hits, but the command went on to
  push `ss-3.1-p3.1` and start CI. The branch and its run were deleted
  minutes later, and the two commits rebuilt with neutral names of the same
  lengths; the scan now stops a push unless it finds nothing. GitHub still
  serves the two old commits to anyone with their ids, until GitHub Support
  removes them, which only the owner can ask for.
- **At 52 columns a row cannot keep its notes on one line.** The columns
  before the folder take 34 of the 49 beside the rail, so the notes go on a
  second line, as the second steps above allow.
- **A long title is dropped even on a wide terminal** when the row would
  otherwise wrap, as at 100 columns, where it wrapped before.

Still to confirm by hand: the picker on the owner's Mac terminal, with an
agent that has a send scheduled.

### P4 — Release v0.1.1

#### Steps

1. Push `ss-3.1`, merged into `main`, on the owner's word; then apply P3's
   settings, the commands below.
   → verify: the ruleset lists the four checks and the admin bypass; the
   repository allows auto-merge and requires pinned actions; CI on `main`,
   started by hand, passes under them.
2. `tools/release 0.1.1`; push the commit and the tag on the owner's word.
   → verify: the push shows the bypass notice; every release job passes;
   `tools/bump-tap` writes the tap and reads it back; Homebrew on macOS and
   on Ubuntu prints `schedule-send 0.1.1`.
3. As built, once Dependabot's first pull request has merged itself.
   → verify: zero private-term hits.

P3's settings. Role 5 is the repository admin role, the one that may bypass:

```sh
gh api -X POST repos/dj-skn/schedule-send/rulesets --input - <<'JSON'
{
  "name": "main requires CI",
  "target": "branch",
  "enforcement": "active",
  "conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}},
  "bypass_actors": [{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always"}],
  "rules": [{"type": "required_status_checks", "parameters": {
    "strict_required_status_checks_policy": false,
    "required_status_checks": [{"context": "Linux"}, {"context": "Linux ARM"},
                               {"context": "macOS"}, {"context": "Vulnerabilities"}]}}]
}
JSON
gh api -X PATCH repos/dj-skn/schedule-send -F allow_auto_merge=true
gh api -X PUT repos/dj-skn/schedule-send/actions/permissions \
  -F enabled=true -f allowed_actions=all -F sha_pinning_required=true
```

#### Step 1 as done — 2026-11-03

The owner ran step 1 ahead of P3.1, with the commands above in one-line
form: `ss-3.1` merged into `main` as `8c9ef26` and pushed, then the three
settings. The release waits for P3.1.

- The merge commit carries the noreply address, and `main` has zero
  private-term hits. The hook installed the new build in the main checkout.
- GitHub stores the ruleset as active, with role 5 bypassing always and the
  four checks required. Auto-merge is allowed, and pinned actions are
  required.
- CI on `main`, started by hand, passed on Linux, Linux ARM, and macOS, and
  the scan: the first run with pinned actions required.
- Dependabot ran on its new configuration within a minute and opened pull
  request #1, checkout v7.0.0 to v7.0.1, moving each pinned commit and its
  version comment together. The auto-merge workflow turned on a squash
  merge. macOS failed once: the runner could not download setup-go's
  archive at its pinned commit, after one attempt, though the other jobs
  and the run on `main` had downloaded it. The archive answered when
  checked, and one rerun of that job passed. GitHub then merged the pull
  request itself, as `ee9d7d2` by Dependabot, 38 seconds after its last
  check passed.
- No CI ran on `main` for `ee9d7d2`: GitHub starts no workflow from a push
  made with a workflow's own token. The pull request's checks, run on the
  merged result, were its test.

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
