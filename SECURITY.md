# Security policy

## Supported versions

The latest release receives security fixes.

## Reporting a vulnerability

Please report security issues privately through
[GitHub's private vulnerability reporting](https://github.com/dj-skn/schedule-send/security/advisories/new)
rather than opening a public issue.

You can expect an acknowledgement within a week. If the report is confirmed, a
fix and an advisory will follow.

## Scope

This tool types text and key presses into terminal panes at a time you chose,
as your own user. The most likely issues are a job reaching a pane it was not
scheduled for, text from a state file or a path being run as something other
than what was written, and anything that lets another local user read or change
a pending job. State and messages are kept under `~/.local/state/herdr-send`
with the directory at mode 700 and new files at mode 600.

This tool does not talk to the network. It runs `herdr`, `systemctl`,
`journalctl`, and `date`, and nothing else.
