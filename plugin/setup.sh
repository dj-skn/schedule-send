#!/bin/sh
# The plugin's install step, which Herdr runs once when the plugin is
# installed from GitHub. The plugin is a front for the schedule-send program,
# so this makes sure the program is there, and leaves an existing install
# alone.
set -eu
cd "$(dirname "$0")/.."
. plugin/find.sh

bin="$(find_schedule_send)"
if [ -n "$bin" ]; then
  echo "schedule-send is already installed: $bin ($("$bin" --version 2> /dev/null || echo 'version unknown'))"
  exit 0
fi
case "$(uname -s)" in
  Linux)
    exec sh install.sh
    ;;
  Darwin)
    echo "schedule-send is not installed. On macOS install it with Homebrew, then install this plugin again:" >&2
    echo "  brew install dj-skn/tap/schedule-send" >&2
    exit 1
    ;;
  *)
    echo "schedule-send runs on Linux and macOS, not on $(uname -s)." >&2
    exit 1
    ;;
esac
