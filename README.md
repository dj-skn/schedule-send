# schedule-send

[![CI](https://github.com/dj-skn/schedule-send/actions/workflows/ci.yml/badge.svg)](https://github.com/dj-skn/schedule-send/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/dj-skn/schedule-send)](https://github.com/dj-skn/schedule-send/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Herdr plugin](https://img.shields.io/badge/Herdr-plugin-blueviolet)](https://herdr.dev/plugins/)

Schedule a message into an AI agent in [Herdr](https://herdr.dev): at a time
you choose, or when its usage limit resets.

![schedule-send in a terminal: the picker, then Enter at each question](docs/media/resume.gif)

`schedule-send` sends a message, a file, or key presses to an agent or a shell
pane. When Claude Code or Codex stops at its usage limit, run it. It reads the
reset time from the screen and schedules `resume` for one minute after. Each
job runs from a timer, so you can close the terminal.

## Install

You need Herdr on Linux or macOS.

With Homebrew, on macOS or Linux:

```sh
brew install dj-skn/tap/schedule-send
```

With the install script, on Linux. It checks the download's SHA-256 and
installs to `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/dj-skn/schedule-send/main/install.sh | sh
```

As a [Herdr plugin](#herdr-plugin), with Herdr 0.9.3 or newer:

```sh
herdr plugin install dj-skn/schedule-send
```

On Linux, the plugin installs the program if it is missing. On macOS, run the
Homebrew command first.

To upgrade, run the Homebrew or install-script command again. On Linux, jobs
fire only while you are logged in, unless
[user lingering](docs/usage.md#requirements-and-the-installed-path) is on.

## Quick start

### Resume after a usage limit

Run it with no options:

```sh
schedule-send
```

It asks which agent, what to send, and when. For an agent at its usage limit,
Enter accepts each proposal. Otherwise, type a time, such as `18:30` or
`+30 min`.

### Send a message at a set time

Choose the agent by its working directory:

```sh
schedule-send --cwd ~/projects/website --at 18:30 --message 'run the e2e suite'
```

![Scheduling a message for the Codex agent in ~/projects/website, then listing it beside an earlier resume job](docs/media/schedule.gif)

### More examples

```sh
# List agents and panes, with their IDs
schedule-send --targets

# Take the proposal without asking
schedule-send --pane wA:p1 --auto

# Press Ctrl-C in a pane at 9:00
schedule-send --pane wA:p3 --at 9:00 --keys ctrl+c

# Send a file as one prompt, an hour from now
schedule-send --pane wA:p2 --at +1h --message-file plan.md

# List scheduled jobs, then cancel one by its ID
schedule-send --list
schedule-send --cancel 20261015-183000-wBp1
```

Add `--dry-run` to preview a job without scheduling it, or `--json` for output
that scripts can read. The [usage reference](docs/usage.md) lists every
option.

## Herdr plugin

Bind the plugin's action to any free key in Herdr's `config.toml`, for
example:

```toml
[[keys.command]]
key = "prefix+alt+s"
type = "plugin_action"
command = "dj-skn.schedule-send.schedule"
description = "schedule a send"
```

Focus an agent or a shell pane and press the key. An overlay asks what to send
and when, with the same proposals as a terminal run.

![The overlay over a Claude Code agent with no usage limit, reading "+30 min" back as a time](docs/media/plugin.png)

## How it works

1. **Schedule.** You pick the agent when asked, or name it with `--cwd`,
   `--pane`, or `--kind`, and choose what to send and when.
2. **Wait.** A systemd user timer on Linux, or launchd on macOS, holds the
   job until its time.
3. **Deliver.** `schedule-send` checks that the pane still holds the same
   agent in the same directory. It waits up to 15 minutes while the agent is
   busy, sends the text once, and confirms the agent took it. If the target
   changed or stays busy, nothing is sent. Any failure raises a Herdr
   notification, unless you pass `--quiet`.

A job missed while the machine was off is skipped, unless it was scheduled
with `--persistent`. The usage reference covers
[missed jobs](docs/usage.md#manage-jobs-and-missed-runs) and
[delivery](docs/usage.md#delivery-and-failures) in full.

## Documentation

- [Usage reference](docs/usage.md): targets, times, delivery, managing jobs,
  output for scripts, the plugin, and where files live.
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md): building from source, the checks, and
  releasing.
- [Security policy](SECURITY.md)

## License

[MIT](LICENSE)
