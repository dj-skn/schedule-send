# Sourced by the plugin's scripts. Herdr starts them with its server's PATH,
# which may lack the directory schedule-send was installed in, so the places
# each installer uses are tried as well.

# find_schedule_send prints the program's path, or nothing.
find_schedule_send() {
  if command -v schedule-send > /dev/null 2>&1; then
    command -v schedule-send
    return
  fi
  # SCHEDULE_SEND_PLUGIN_PLACES replaces the list, for the plugin's own test.
  places="${SCHEDULE_SEND_PLUGIN_PLACES-$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/home/linuxbrew/.linuxbrew/bin}"
  old_ifs=$IFS
  IFS=:
  for place in $places; do
    if [ -x "$place/schedule-send" ]; then
      IFS=$old_ifs
      echo "$place/schedule-send"
      return
    fi
  done
  IFS=$old_ifs
}
