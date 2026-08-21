#!/bin/zsh
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  schedule-send [--pane PANE] --at TIME [--message TEXT] [options]

Options:
  --pane ID              Herdr pane id, for example wJ:p1. If omitted,
                         auto-selects the only pane or asks when multiple exist.
  --at TIME              Local time. Accepts "HH:MM", "HH:MM:SS",
                         "YYYY-MM-DD HH:MM:SS", or "YYYY-MM-DDTHH:MM:SS".
                         Time-only values that already passed roll to tomorrow.
  --message TEXT         Text to send before the final Enter.
  --message-file PATH    Read message text from a file.
  --enter-only           Schedule only an Enter key send.
  --no-initial-enter     Do not send Enter before the message.
  --log PATH             Log file. Default: /tmp/herdr-scheduled-send-<pane>-<timestamp>.log.
  --dry-run              Print what would be scheduled and exit.
  -h, --help             Show this help.

Examples:
  schedule-send --at 15:50 --message "resume and commit when done"
  schedule-send --pane wJ:p1 --at 03:30 --message-file prompt.txt
  schedule-send --at "2026-06-24 15:50:00" --enter-only

Environment:
  HERDR_SCHEDULE_NOW_EPOCH can override the current epoch for dry-run testing.
EOF
}

die() {
  print -u2 "$*"
  exit 1
}

have_gnu_date() {
  date --version >/dev/null 2>&1
}

parse_epoch() {
  local value="$1"
  if have_gnu_date; then
    date -d "$value" +%s 2>/dev/null || true
  else
    date -j -f '%Y-%m-%d %H:%M:%S' "$value" +%s 2>/dev/null || true
  fi
}

format_epoch() {
  local epoch="$1"
  local format="$2"
  if have_gnu_date; then
    date -d "@$epoch" "$format"
  else
    date -r "$epoch" "$format"
  fi
}

pane_rows() {
  jq -r '
    .result.panes[]
    | [
        .pane_id,
        (.agent // "terminal"),
        (.agent_status // "unknown"),
        (if .focused then "focused" else "-" end),
        (.cwd // .foreground_cwd // "-")
      ]
    | @tsv
  '
}

choose_pane() {
  command -v jq >/dev/null 2>&1 || die "jq is required for pane auto-selection. Install jq or pass --pane."

  local panes_json
  panes_json="$(herdr pane list)"

  local pane_count
  pane_count="$(print -r -- "$panes_json" | jq '.result.panes | length')"
  [[ "$pane_count" == <-> ]] || die "Could not parse Herdr pane list."

  if (( pane_count == 0 )); then
    die "No Herdr panes found."
  fi

  if (( pane_count == 1 )); then
    print -r -- "$panes_json" | jq -r '.result.panes[0].pane_id'
    return
  fi

  local tty_fd
  if [[ ! -t 0 ]]; then
    print -u2 "Multiple Herdr panes found. Pass --pane explicitly in non-interactive mode."
    print -r -- "$panes_json" | pane_rows >&2
    exit 1
  fi
  exec {tty_fd}<>/dev/tty || die "Could not open /dev/tty for pane selection."

  local -a rows pane_ids
  rows=("${(@f)$(print -r -- "$panes_json" | pane_rows)}")

  print -u2 "Multiple Herdr panes found. Choose a target:"
  printf '%s\n' " #  Pane      Agent       Status      Focus    CWD" >&$tty_fd

  local i line pane_id agent pane_status focus cwd
  for (( i = 1; i <= ${#rows}; i++ )); do
    line="${rows[$i]}"
    IFS=$'\t' read -r pane_id agent pane_status focus cwd <<< "$line"
    pane_ids[$i]="$pane_id"
    printf '%2d) %-9s %-11s %-11s %-8s %s\n' "$i" "$pane_id" "$agent" "$pane_status" "$focus" "$cwd" >&$tty_fd
  done

  local choice
  while true; do
    printf 'Pane number: ' >&$tty_fd
    IFS= read -r choice <&$tty_fd
    if [[ "$choice" == <-> ]] && (( choice >= 1 && choice <= ${#pane_ids} )); then
      exec {tty_fd}>&-
      print -r -- "${pane_ids[$choice]}"
      return
    fi
    print -u2 "Enter a number from 1 to ${#pane_ids}."
  done
}

pane=""
at=""
message=""
message_file=""
log_path=""
initial_enter=1
enter_only=0
dry_run=0

while (( $# > 0 )); do
  case "$1" in
    --pane)
      pane="${2:-}"
      shift 2
      ;;
    --at)
      at="${2:-}"
      shift 2
      ;;
    --message)
      message="${2:-}"
      shift 2
      ;;
    --message-file)
      message_file="${2:-}"
      shift 2
      ;;
    --log)
      log_path="${2:-}"
      shift 2
      ;;
    --enter-only)
      enter_only=1
      shift
      ;;
    --no-initial-enter)
      initial_enter=0
      shift
      ;;
    --dry-run)
      dry_run=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      print -u2 "Unknown argument: $1"
      usage >&2
      exit 2
      ;;
  esac
done

[[ -n "$at" ]] || { usage >&2; die "Missing required --at."; }

if [[ -n "$message" && -n "$message_file" ]]; then
  die "Use either --message or --message-file, not both."
fi

if [[ -n "$message_file" ]]; then
  [[ -f "$message_file" ]] || die "Message file does not exist: $message_file"
  message="$(<"$message_file")"
fi

if (( enter_only == 0 )) && [[ -z "$message" ]]; then
  die "Missing --message. Use --enter-only to schedule only Enter."
fi

command -v herdr >/dev/null 2>&1 || die "herdr is not on PATH."
command -v date >/dev/null 2>&1 || die "date is not on PATH."
command -v zsh >/dev/null 2>&1 || die "zsh is not on PATH."

if [[ -z "$pane" ]]; then
  pane="$(choose_pane)"
fi

now_epoch="${HERDR_SCHEDULE_NOW_EPOCH:-$(date +%s)}"
[[ "$now_epoch" == <-> ]] || die "HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch."

normalized_at="${at/T/ }"
time_only=0

if [[ "$normalized_at" =~ '^[0-9]{1,2}:[0-9]{2}$' ]]; then
  time_only=1
  normalized_at="$(format_epoch "$now_epoch" +%Y-%m-%d) $(printf '%02d:%s:00' "${normalized_at%%:*}" "${normalized_at#*:}")"
elif [[ "$normalized_at" =~ '^[0-9]{1,2}:[0-9]{2}:[0-9]{2}$' ]]; then
  time_only=1
  hour="${normalized_at%%:*}"
  rest="${normalized_at#*:}"
  normalized_at="$(format_epoch "$now_epoch" +%Y-%m-%d) $(printf '%02d:%s' "$hour" "$rest")"
fi

run_at="$(parse_epoch "$normalized_at")"
[[ -n "$run_at" ]] || die "Could not parse --at '$at'. Use HH:MM, HH:MM:SS, or YYYY-MM-DD HH:MM:SS."

if (( time_only == 1 && run_at <= now_epoch )); then
  run_at=$((run_at + 86400))
  normalized_at="$(format_epoch "$run_at" '+%Y-%m-%d %H:%M:%S')"
fi

delay=$((run_at - now_epoch))
if (( delay < 0 )); then
  die "Target time has already passed: $normalized_at"
fi

if [[ -z "$log_path" ]]; then
  safe_pane="${pane//[^A-Za-z0-9_.-]/_}"
  stamp="$(format_epoch "$run_at" '+%Y%m%d-%H%M%S')"
  log_path="/tmp/herdr-scheduled-send-${safe_pane}-${stamp}.log"
fi

inner='
set -euo pipefail
sleep "$1"
if [[ "$4" == "1" ]]; then
  herdr pane send-keys "$2" enter
  sleep 0.5
fi
if [[ "$5" != "1" ]]; then
  herdr pane send-text "$2" "$3"
  herdr pane send-keys "$2" enter
fi
'

if (( dry_run == 1 )); then
  print "pane=$pane"
  print "at=$normalized_at"
  print "run_at_epoch=$run_at"
  print "delay_seconds=$delay"
  print "initial_enter=$initial_enter"
  print "enter_only=$enter_only"
  print "log=$log_path"
  exit 0
fi

if command -v setsid >/dev/null 2>&1; then
  nohup setsid zsh -lc "$inner" _ "$delay" "$pane" "$message" "$initial_enter" "$enter_only" >"$log_path" 2>&1 &
elif command -v caffeinate >/dev/null 2>&1; then
  nohup caffeinate -i zsh -lc "$inner" _ "$delay" "$pane" "$message" "$initial_enter" "$enter_only" >"$log_path" 2>&1 &
else
  nohup zsh -lc "$inner" _ "$delay" "$pane" "$message" "$initial_enter" "$enter_only" >"$log_path" 2>&1 &
fi
pid=$!

print "scheduled_pid=$pid"
print "pane=$pane"
print "at=$normalized_at"
print "run_at_epoch=$run_at"
print "delay_seconds=$delay"
print "log=$log_path"
