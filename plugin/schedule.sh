#!/bin/sh
# The plugin's action. Herdr runs it with the focused pane's id in
# HERDR_PANE_ID. It opens the prompt pane over that pane and tells the prompt
# which pane it is for.
set -eu
exec "${HERDR_BIN_PATH:-herdr}" plugin pane open \
  --plugin "${HERDR_PLUGIN_ID:-dj-skn.schedule-send}" \
  --entrypoint prompt \
  --placement overlay \
  --env "SCHEDULE_SEND_TARGET=${HERDR_PANE_ID:-}" \
  --focus
