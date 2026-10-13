#!/bin/sh
# Runs in the plugin's pane. It schedules a send to the pane the action was
# invoked on: with a usage-limit notice on its screen, schedule-send proposes
# the reset time; otherwise it asks for one. The pane closes when this script
# ends, so it waits for Enter and the receipt can be read.
set -u
. "$(dirname "$0")/find.sh"

finish() {
  printf '\nPress Enter to close.'
  read -r _ || true
}
trap finish EXIT

bin="$(find_schedule_send)"
if [ -z "$bin" ]; then
  echo "schedule-send is not installed."
  echo "On macOS:  brew install dj-skn/tap/schedule-send"
  echo "On Linux:  curl -fsSL https://raw.githubusercontent.com/dj-skn/schedule-send/main/install.sh | sh"
  exit 1
fi

# Options that apply to every send from the plugin, one or more per line, in
# a file named "args" in the plugin's configuration directory. Lines that
# start with # are comments.
set --
args="${HERDR_PLUGIN_CONFIG_DIR:-}/args"
if [ -n "${HERDR_PLUGIN_CONFIG_DIR:-}" ] && [ -f "$args" ]; then
  # shellcheck disable=SC2046
  set -- $(grep -v '^[[:space:]]*#' "$args")
fi
if [ -n "${SCHEDULE_SEND_TARGET:-}" ]; then
  set -- --pane "$SCHEDULE_SEND_TARGET" "$@"
fi
"$bin" "$@"
