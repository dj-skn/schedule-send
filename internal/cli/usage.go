package cli

const usage = `Usage:
  schedule-send --at TIME (--message TEXT | --message-file PATH | --keys KEYS) [options]
  schedule-send [--auto] [--message TEXT | ...] [options]
  schedule-send --list
  schedule-send --cancel ID
  schedule-send --status ID

Timing:
  --at TIME              Local time. "HH:MM", "HH:MM:SS", "12:27 AM", "3pm",
                         "YYYY-MM-DD HH:MM:SS" or "YYYY-MM-DDTHH:MM:SS".
                         Time-only values that already passed roll to
                         tomorrow, correctly across DST. Omitted: read the
                         reset time from the target's usage-limit notice, add
                         one minute, and confirm it on the terminal.
  --auto                 Take the detected reset time without confirming, for
                         runs with no terminal. Fails if no notice names one.
  --persistent           If the machine is down at the target time, fire at the
                         next boot instead of skipping the job.

Payload:
  --message TEXT         Text to submit. Without --at, defaults to "resume".
  --message-file PATH    Read the text from a file. Multi-line is safe.
  --keys KEYS            Space-separated key names, e.g. "esc" or "ctrl+c enter".
  --enter-only           Alias for --keys enter.

Target selection:
  --pane ID              Target this pane or agent pane directly, e.g. wX:p1.
  --cwd PATH             Select by working directory. Prefers agents; falls back
                         to plain panes only when no agent matches.
  --kind NAME            Only match agents of this kind, e.g. claude, codex.
  --panes-only           Only match plain panes, never agents.
  --pick unique|best     Non-interactive tie-break. "unique" (default) fails when
                         more than one target matches. "best" takes the top rank.
  --allow-self           Permit targeting the pane this command runs in.

Delivery:
  --if-busy wait|abort   When the target agent is working, wait up to 15 minutes
                         for it to settle, or abort the send. A wait that runs
                         out fails the send. Default: wait.
  --recover MODE         What to do when the submit is not confirmed and the
                         text is still sitting in the agent's composer.
                         "keys" (default) presses Enter again. "focus" then
                         focuses the pane and presses Enter once more, and
                         hands focus back afterwards. "none" fails instead.
  --submit-grace SECS    How long each confirmation attempt waits for the
                         submit to show up. Default: 4.
  --force                Skip the fire-time check that the target still is what
                         it was when the job was scheduled.

Other:
  --quiet                Do not raise a Herdr notification when a job fails.
  --dry-run              Print the resolved target and the timer, change nothing.
  -h, --help             Show this help.

Examples:
  schedule-send --at 15:50 --message 'resume, then commit when done'
  schedule-send --at 03:30 --cwd ~/projects/website --message-file prompt.txt
  schedule-send --at 09:00 --pane wS:p1 --keys 'ctrl+c'
  schedule-send --cwd ~/projects/website          # resume after its usage limit
  schedule-send --list

Environment:
  HERDR_SCHEDULE_NOW_EPOCH   Override "now" for --dry-run testing.
  NO_COLOR                   Disable colour in the picker.
`
