#!/bin/zsh
#
# schedule-send — schedule text or key presses into a Herdr agent or pane.
#
# Linux / GNU coreutils only. The delayed job is owned by an on-disk
# systemd --user timer, so it survives reboots (this account has lingering
# enabled), is listable, and is cancellable. Selection is re-resolved at
# fire time so a pane that has changed identity is never written to.

set -euo pipefail

readonly SELF="${0:A}"
readonly STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/herdr-send"
readonly UNIT_PREFIX="herdr-send"
readonly UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

# ---------------------------------------------------------------- output ---

typeset -g C_RESET='' C_DIM='' C_RED='' C_GREEN='' C_YELLOW='' C_BOLD=''

init_colors() {
  local fd="$1"
  if [[ -z "${NO_COLOR:-}" ]] && [[ -t "$fd" ]]; then
    C_RESET=$'\e[0m'; C_DIM=$'\e[2m'; C_RED=$'\e[31m'
    C_GREEN=$'\e[32m'; C_YELLOW=$'\e[33m'; C_BOLD=$'\e[1m'
  fi
}

die() { print -u2 -- "$*"; exit 1; }

usage() {
  cat <<'EOF'
Usage:
  schedule-send --at TIME (--message TEXT | --message-file PATH | --keys KEYS) [options]
  schedule-send --list
  schedule-send --cancel ID
  schedule-send --status ID

Timing:
  --at TIME              Local time. "HH:MM", "HH:MM:SS", "YYYY-MM-DD HH:MM:SS"
                         or "YYYY-MM-DDTHH:MM:SS". Time-only values that already
                         passed roll to tomorrow, correctly across DST.
  --persistent           If the machine is down at the target time, fire at the
                         next boot instead of skipping the job.

Payload:
  --message TEXT         Text to submit.
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
  --if-busy wait|abort   When the target agent is working, wait for it to settle
                         or abort the send. Default: wait.
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
  schedule-send --list

Environment:
  HERDR_SCHEDULE_NOW_EPOCH   Override "now" for --dry-run testing.
  NO_COLOR                   Disable colour in the picker.
EOF
}

# ------------------------------------------------------------ herdr access ---

typeset -g HERDR_BIN=''

resolve_herdr() {
  if [[ -n "${HERDR_BIN_OVERRIDE:-}" ]]; then
    HERDR_BIN="$HERDR_BIN_OVERRIDE"
  else
    HERDR_BIN="$(command -v herdr 2>/dev/null || true)"
  fi
  [[ -n "$HERDR_BIN" && -x "$HERDR_BIN" ]] || die "herdr is not on PATH."
  HERDR_BIN="${HERDR_BIN:A}"
}

# Emits TSV: kind \t pane_id \t agent \t status \t cwd \t title
# Every field is guaranteed non-empty so tab splitting stays positional.
list_targets() {
  "$HERDR_BIN" agent list | jq -r '
    .result.agents[]
    | ["agent", .pane_id, (.agent // "agent"), (.agent_status // "unknown"),
       (.cwd // .foreground_cwd // "-"),
       (if (.terminal_title_stripped // "") == "" then "-"
        else .terminal_title_stripped end)]
    | @tsv'
  "$HERDR_BIN" pane list | jq -r '
    .result.panes[]
    | select(.agent == null)
    | ["pane", .pane_id, "-", "-",
       (.cwd // .foreground_cwd // "-"),
       (if (.terminal_title_stripped // "") == "" then "-"
        else .terminal_title_stripped end)]
    | @tsv'
}

# Foreground process name for a plain pane, for the picker label.
pane_proc() {
  "$HERDR_BIN" pane process-info --pane "$1" 2>/dev/null \
    | jq -r '.result.process_info.foreground_processes[0].name // "-"' 2>/dev/null \
    || true
}

row_field() { local -a f; f=("${(@ps:\t:)1}"); print -r -- "${f[$2]}"; }

# --------------------------------------------------------------- selection ---

norm_path() {
  local p="${~1}"
  p="${p:A}"
  print -r -- "${p%/}"
}

cwd_matches() {
  local target="$1" want="$2"
  [[ "$target" == "$want" || "$target" == "$want"/* ]]
}

rank_of() {
  local kind="$1" st="$2"
  if [[ "$kind" != agent ]]; then print -r -- 1; return; fi
  case "$st" in
    idle|done) print -r -- 10 ;;
    unknown)   print -r -- 6 ;;
    working)   print -r -- 4 ;;
    blocked)   print -r -- 2 ;;
    *)         print -r -- 3 ;;
  esac
}

status_glyph() {
  local kind="$1" st="$2"
  if [[ "$kind" != agent ]]; then print -r -- "${C_DIM}·${C_RESET}"; return; fi
  case "$st" in
    idle)    print -r -- "${C_GREEN}○${C_RESET}" ;;
    done)    print -r -- "${C_GREEN}✓${C_RESET}" ;;
    working) print -r -- "${C_YELLOW}●${C_RESET}" ;;
    blocked) print -r -- "${C_RED}◐${C_RESET}" ;;
    *)       print -r -- "${C_DIM}?${C_RESET}" ;;
  esac
}

pretty_cwd() {
  local p="$1"
  [[ "$p" == "$HOME" ]] && p="~"
  [[ "$p" == "$HOME"/* ]] && p="~${p#$HOME}"
  print -r -- "$p"
}

# Sorts rows best-first. Ties break deterministically on kind then pane id.
sort_rows() {
  local row
  for row in "$@"; do
    printf '%02d\t%s\n' "$(rank_of "$(row_field "$row" 1)" "$(row_field "$row" 4)")" "$row"
  done | sort -t$'\t' -k1,1r -k3,3 | cut -f2-
}

filter_rows() {
  local -a out=()
  local row kind pane cwd
  for row in "${(@)ROWS}"; do
    kind="$(row_field "$row" 1)"
    pane="$(row_field "$row" 2)"
    cwd="$(row_field "$row" 5)"

    [[ -n "$opt_kind" && "$kind" != agent ]] && continue
    [[ -n "$opt_kind" && "$(row_field "$row" 3)" != "$opt_kind" ]] && continue
    (( opt_panes_only )) && [[ "$kind" != pane ]] && continue
    if [[ -n "$opt_cwd" ]]; then
      cwd_matches "$(norm_path "$cwd")" "$opt_cwd" || continue
    fi
    (( opt_allow_self )) || [[ "$pane" != "${HERDR_PANE_ID:-}" ]] || continue
    out+=("$row")
  done

  # A cwd selector that hits a real agent should not stay ambiguous just
  # because plain panes share the directory.
  if [[ -n "$opt_cwd" ]] && (( ! opt_panes_only )); then
    local -a agents_only=()
    for row in "${(@)out}"; do
      [[ "$(row_field "$row" 1)" == agent ]] && agents_only+=("$row")
    done
    (( ${#agents_only} > 0 )) && out=("${(@)agents_only}")
  fi

  print -rl -- "${(@)out}"
}

render_picker() {
  local fd="$1"
  shift
  local -a rows=("$@")
  local i row kind pane agent st cwd title label glyph mark

  print -r -- "" >&$fd
  print -r -- "${C_BOLD}agents${C_RESET}" >&$fd
  local any=0
  for (( i = 1; i <= ${#rows}; i++ )); do
    row="${rows[$i]}"
    [[ "$(row_field "$row" 1)" == agent ]] || continue
    any=1
    pane="$(row_field "$row" 2)"; agent="$(row_field "$row" 3)"
    st="$(row_field "$row" 4)"; cwd="$(row_field "$row" 5)"
    title="$(row_field "$row" 6)"
    glyph="$(status_glyph agent "$st")"
    mark=""
    [[ "$pane" == "${HERDR_PANE_ID:-}" ]] && mark=" ${C_DIM}← this pane${C_RESET}"
    [[ "$title" == "-" ]] && title=""
    printf '%3d  %s %-8s %-8s %-8s %-26s %s%s\n' \
      "$i" "$glyph" "$st" "$agent" "$pane" "$(pretty_cwd "$cwd")" \
      "${C_DIM}${title}${C_RESET}" "$mark" >&$fd
  done
  (( any )) || print -r -- "     ${C_DIM}none${C_RESET}" >&$fd

  print -r -- "" >&$fd
  print -r -- "${C_BOLD}panes${C_RESET}" >&$fd
  any=0
  for (( i = 1; i <= ${#rows}; i++ )); do
    row="${rows[$i]}"
    [[ "$(row_field "$row" 1)" == pane ]] || continue
    any=1
    pane="$(row_field "$row" 2)"; cwd="$(row_field "$row" 5)"
    label="$(pane_proc "$pane")"
    [[ -z "$label" ]] && label="-"
    printf '%3d  %s %-8s %-8s %s\n' \
      "$i" "${C_DIM}·${C_RESET}" "$label" "$pane" "$(pretty_cwd "$cwd")" >&$fd
  done
  (( any )) || print -r -- "     ${C_DIM}none${C_RESET}" >&$fd
  print -r -- "" >&$fd
}

choose_target() {
  local -a rows
  rows=("${(@f)$(filter_rows)}")
  [[ -n "${rows[1]:-}" ]] || rows=()

  (( ${#rows} > 0 )) || die "No target matches the given selectors."

  if (( ${#rows} == 1 )); then
    print -r -- "${rows[1]}"
    return
  fi

  rows=("${(@f)$(sort_rows "${(@)rows}")}")

  if [[ ! -t 0 ]]; then
    if [[ "$opt_pick" == best ]]; then
      print -u2 -- "Multiple targets matched; picking $(row_field "${rows[1]}" 2)."
      print -r -- "${rows[1]}"
      return
    fi
    print -u2 -- "Multiple targets matched. Narrow with --cwd/--kind/--pane, or use --pick best."
    init_colors 2
    render_picker 2 "${(@)rows}"
    exit 1
  fi

  local tty_fd
  exec {tty_fd}<>/dev/tty || die "Could not open /dev/tty for target selection."
  init_colors "$tty_fd"
  render_picker "$tty_fd" "${(@)rows}"

  local choice
  while true; do
    printf 'target [1]: ' >&$tty_fd
    IFS= read -r choice <&$tty_fd || choice=""
    [[ -z "$choice" ]] && choice=1
    if [[ "$choice" == <-> ]] && (( choice >= 1 && choice <= ${#rows} )); then
      exec {tty_fd}>&-
      print -r -- "${rows[$choice]}"
      return
    fi
    print -r -- "Enter a number from 1 to ${#rows}." >&$tty_fd
  done
}

# ------------------------------------------------------------------- time ---

parse_epoch() { date -d "$1" +%s 2>/dev/null || true; }
fmt_epoch()   { date -d "@$1" "$2"; }

# ------------------------------------------------------------ subcommands ---

cmd_list() {
  init_colors 1
  local f id at target msg unit state
  local -a files=("$STATE_DIR"/*.json(N))
  if (( ${#files} == 0 )); then
    print -r -- "No scheduled sends."
    return
  fi
  printf '%-24s %-20s %-10s %-9s %s\n' ID AT TARGET STATE PAYLOAD
  for f in "${(@)files}"; do
    id="$(jq -r '.id' "$f")"
    at="$(jq -r '.at' "$f")"
    target="$(jq -r '.pane_id' "$f")"
    unit="${UNIT_PREFIX}-${id}.timer"
    state="$(systemctl --user is-active "$unit" 2>/dev/null || true)"
    [[ -z "$state" ]] && state="gone"
    if [[ "$(jq -r '.mode' "$f")" == keys ]]; then
      msg="keys: $(jq -r '.keys' "$f")"
    else
      msg="$(head -c 60 "${f%.json}.msg" 2>/dev/null | head -1 || true)"
    fi
    printf '%-24s %-20s %-10s %-9s %s\n' "$id" "$at" "$target" "$state" "$msg"
  done
}

cmd_cancel() {
  local id="$1"
  local unit="${UNIT_PREFIX}-${id}"
  [[ -f "$STATE_DIR/$id.json" ]] || die "No scheduled send with id: $id"
  systemctl --user disable --now "$unit.timer" >/dev/null 2>&1 || true
  systemctl --user stop "$unit.service" >/dev/null 2>&1 || true
  systemctl --user reset-failed "$unit.timer" "$unit.service" >/dev/null 2>&1 || true
  rm -f "$UNIT_DIR/$unit.timer" "$UNIT_DIR/$unit.service"
  systemctl --user daemon-reload >/dev/null 2>&1 || true
  rm -f "$STATE_DIR/$id.json" "$STATE_DIR/$id.msg"
  print -r -- "cancelled $id"
}

cmd_status() {
  local id="$1"
  local unit="${UNIT_PREFIX}-${id}"
  if [[ -f "$STATE_DIR/$id.json" ]]; then
    jq . "$STATE_DIR/$id.json"
  else
    print -u2 -- "No state file for $id (job may have already run)."
  fi
  print -r -- ""
  systemctl --user status "$unit.timer" --no-pager 2>/dev/null || true
  print -r -- ""
  journalctl --user -u "$unit.service" --no-pager -n 40 2>/dev/null || true
}

# ----------------------------------------------------------------- worker ---

typeset -g W_ID='' W_QUIET=0

worker_notify() {
  (( W_QUIET )) && return 0
  [[ -n "$HERDR_BIN" ]] || return 0
  "$HERDR_BIN" notification show "Scheduled send failed" \
    --body "$1" --sound request >/dev/null 2>&1 || true
}

worker_cleanup() {
  rm -f "$STATE_DIR/$W_ID.json" "$STATE_DIR/$W_ID.msg"
  # A one-shot timer must not linger in timers.target.wants, or it would be
  # re-armed at the next boot.
  systemctl --user disable "${UNIT_PREFIX}-${W_ID}.timer" >/dev/null 2>&1 || true
  rm -f "$UNIT_DIR/${UNIT_PREFIX}-${W_ID}.timer" \
        "$UNIT_DIR/${UNIT_PREFIX}-${W_ID}.service"
  systemctl --user daemon-reload >/dev/null 2>&1 || true
}

worker_fail() {
  print -u2 -- "$1"
  worker_notify "$1"
  worker_cleanup
  exit 1
}

cmd_fire() {
  W_ID="$1"
  local state="$STATE_DIR/$W_ID.json"
  [[ -f "$state" ]] || die "No state file for $W_ID"

  local pane kind agent_kind cwd mode keys wait_policy force quiet
  pane="$(jq -r '.pane_id' "$state")"
  kind="$(jq -r '.kind' "$state")"
  agent_kind="$(jq -r '.agent' "$state")"
  cwd="$(jq -r '.cwd' "$state")"
  mode="$(jq -r '.mode' "$state")"
  keys="$(jq -r '.keys' "$state")"
  wait_policy="$(jq -r '.if_busy' "$state")"
  force="$(jq -r '.force' "$state")"
  quiet="$(jq -r '.quiet' "$state")"
  W_QUIET=$(( quiet == 1 ? 1 : 0 ))

  HERDR_BIN="$(jq -r '.herdr_bin' "$state")"
  [[ -x "$HERDR_BIN" ]] || HERDR_BIN="$(command -v herdr 2>/dev/null || true)"
  [[ -x "$HERDR_BIN" ]] || worker_fail "herdr binary is unavailable at fire time."

  # Re-resolve: the target must still be what it was when scheduled.
  if (( force != 1 )); then
    local -a rows
    rows=("${(@f)$(list_targets)}")
    local row found=""
    for row in "${(@)rows}"; do
      [[ "$(row_field "$row" 2)" == "$pane" ]] || continue
      found="$row"
      break
    done
    [[ -n "$found" ]] || worker_fail "Target pane $pane no longer exists; send aborted."
    [[ "$(row_field "$found" 1)" == "$kind" ]] \
      || worker_fail "Pane $pane is no longer a $kind; send aborted."
    if [[ "$kind" == agent ]]; then
      [[ "$(row_field "$found" 3)" == "$agent_kind" ]] \
        || worker_fail "Pane $pane now runs $(row_field "$found" 3), not $agent_kind; send aborted."
    fi
    [[ "$(row_field "$found" 5)" == "$cwd" ]] \
      || worker_fail "Pane $pane moved to $(row_field "$found" 5); send aborted."
  fi

  if [[ "$mode" == keys ]]; then
    local -a keylist=(${=keys})
    # agent send-keys validates agent identity first; pane send-keys is the
    # right surface only for a plain terminal.
    local key_surface=pane
    [[ "$kind" == agent ]] && key_surface=agent
    "$HERDR_BIN" "$key_surface" send-keys "$pane" "${(@)keylist}" >/dev/null \
      || worker_fail "send-keys to $pane failed."
    print -r -- "sent keys to $pane: $keys"
    worker_cleanup
    return 0
  fi

  local msg
  msg="$(<"$STATE_DIR/$W_ID.msg")"

  if [[ "$kind" == agent ]]; then
    if [[ "$wait_policy" == abort ]]; then
      local status_now
      status_now="$("$HERDR_BIN" agent get "$pane" | jq -r '.result.agent.agent_status')"
      [[ "$status_now" == idle || "$status_now" == done ]] \
        || worker_fail "Agent in $pane is $status_now and --if-busy=abort was set."
    else
      "$HERDR_BIN" agent wait "$pane" --timeout 900000 >/dev/null 2>&1 || true
    fi
    "$HERDR_BIN" agent prompt "$pane" "$msg" --wait --timeout 900000 >/dev/null \
      || worker_fail "agent prompt to $pane did not complete cleanly; the text may still have been submitted."
    print -r -- "prompted agent in $pane"
  else
    "$HERDR_BIN" pane run "$pane" "$msg" >/dev/null \
      || worker_fail "pane run in $pane failed."
    print -r -- "ran text in $pane"
  fi

  worker_cleanup
}

# ------------------------------------------------------------------- main ---

if [[ "${1:-}" == __fire ]]; then
  [[ -n "${2:-}" ]] || die "__fire needs an id"
  cmd_fire "$2"
  exit 0
fi

opt_pane=""; opt_cwd=""; opt_kind=""; opt_at=""
opt_message=""; opt_message_file=""; opt_keys=""
opt_pick="unique"; opt_if_busy="wait"
opt_panes_only=0; opt_allow_self=0; opt_force=0
opt_quiet=0; opt_dry_run=0; opt_persistent=0
have_message=0

flag_value() {
  local flag="$1" val="${2-}"
  [[ -n "$val" ]] || die "Missing value for $flag."
  [[ "$val" != --* ]] || die "Missing value for $flag (got the flag '$val')."
  print -r -- "$val"
}

while (( $# > 0 )); do
  case "$1" in
    --pane)         opt_pane="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --cwd)          opt_cwd="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --kind)         opt_kind="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --at)           opt_at="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --message)      opt_message="$(flag_value "$1" "${2-}")"; have_message=1; shift 2 ;;
    --message-file) opt_message_file="$(flag_value "$1" "${2-}")"; have_message=1; shift 2 ;;
    --keys)         opt_keys="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --pick)         opt_pick="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --if-busy)      opt_if_busy="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --enter-only)   opt_keys="enter"; shift ;;
    --panes-only)   opt_panes_only=1; shift ;;
    --allow-self)   opt_allow_self=1; shift ;;
    --force)        opt_force=1; shift ;;
    --quiet)        opt_quiet=1; shift ;;
    --persistent)   opt_persistent=1; shift ;;
    --dry-run)      opt_dry_run=1; shift ;;
    --list)         resolve_herdr; cmd_list; exit 0 ;;
    --cancel)       cmd_cancel "$(flag_value "$1" "${2-}")"; exit 0 ;;
    --status)       cmd_status "$(flag_value "$1" "${2-}")"; exit 0 ;;
    -h|--help)      usage; exit 0 ;;
    *)              print -u2 -- "Unknown argument: $1"; usage >&2; exit 2 ;;
  esac
done

[[ "$opt_pick" == unique || "$opt_pick" == best ]] \
  || die "--pick must be 'unique' or 'best'."
[[ "$opt_if_busy" == wait || "$opt_if_busy" == abort ]] \
  || die "--if-busy must be 'wait' or 'abort'."

[[ -n "$opt_at" ]] || { usage >&2; die "Missing required --at."; }

if [[ -n "$opt_message" && -n "$opt_message_file" ]]; then
  die "Use either --message or --message-file, not both."
fi
if (( have_message )) && [[ -n "$opt_keys" ]]; then
  die "Use either a message or --keys, not both."
fi
if [[ -n "$opt_message_file" ]]; then
  [[ -f "$opt_message_file" ]] || die "Message file does not exist: $opt_message_file"
  opt_message="$(<"$opt_message_file")"
fi
if [[ -z "$opt_keys" && -z "$opt_message" ]]; then
  die "Nothing to send. Pass --message, --message-file, or --keys."
fi

command -v jq >/dev/null 2>&1 || die "jq is required."
command -v systemctl >/dev/null 2>&1 || die "systemctl is required."
# is-system-running exits non-zero for "degraded", which merely means some
# unrelated unit failed earlier. Only a manager we cannot talk to at all is
# a real blocker, and that prints nothing.
[[ -n "$(systemctl --user is-system-running 2>/dev/null || true)" ]] \
  || die "systemd --user is not available for this account."
resolve_herdr

if [[ -n "$opt_cwd" ]]; then
  opt_cwd="$(norm_path "$opt_cwd")"
fi

typeset -ga ROWS
ROWS=("${(@f)$(list_targets)}")
[[ -n "${ROWS[1]:-}" ]] || die "Herdr reported no panes."

if [[ -n "$opt_pane" ]]; then
  target=""
  for row in "${(@)ROWS}"; do
    [[ "$(row_field "$row" 2)" == "$opt_pane" ]] && { target="$row"; break; }
  done
  [[ -n "$target" ]] || die "No such pane: $opt_pane"
else
  target="$(choose_target)"
fi

t_kind="$(row_field "$target" 1)"
t_pane="$(row_field "$target" 2)"
t_agent="$(row_field "$target" 3)"
t_status="$(row_field "$target" 4)"
t_cwd="$(row_field "$target" 5)"

if [[ "$t_pane" == "${HERDR_PANE_ID:-}" ]] && (( ! opt_allow_self )); then
  die "Refusing to target the pane this command runs in. Pass --allow-self to override."
fi
if [[ "$t_kind" == agent && "$t_status" == blocked && -z "$opt_keys" ]]; then
  print -u2 -- "Warning: agent in $t_pane is blocked on a prompt right now."
fi

now_epoch="${HERDR_SCHEDULE_NOW_EPOCH:-$(date +%s)}"
[[ "$now_epoch" == <-> ]] || die "HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch."

normalized_at="${opt_at/T/ }"
time_only=0
hhmmss=""

if [[ "$normalized_at" =~ '^[0-9]{1,2}:[0-9]{2}$' ]]; then
  time_only=1
  hhmmss="$(printf '%02d:%s:00' "${normalized_at%%:*}" "${normalized_at#*:}")"
elif [[ "$normalized_at" =~ '^[0-9]{1,2}:[0-9]{2}:[0-9]{2}$' ]]; then
  time_only=1
  hhmmss="$(printf '%02d:%s' "${normalized_at%%:*}" "${normalized_at#*:}")"
fi

if (( time_only )); then
  today="$(fmt_epoch "$now_epoch" +%F)"
  run_at="$(parse_epoch "$today $hhmmss")"
  [[ -n "$run_at" ]] || die "Could not parse --at '$opt_at'."
  if (( run_at <= now_epoch )); then
    # Re-parse against tomorrow's calendar date rather than adding 86400,
    # so a DST transition between the two days cannot shift the result.
    tomorrow="$(date -d "$today +1 day" +%F)"
    run_at="$(parse_epoch "$tomorrow $hhmmss")"
    [[ -n "$run_at" ]] || die "Could not resolve tomorrow's $hhmmss."
  fi
else
  run_at="$(parse_epoch "$normalized_at")"
  [[ -n "$run_at" ]] || die "Could not parse --at '$opt_at'. Use HH:MM, HH:MM:SS, or YYYY-MM-DD HH:MM:SS."
fi

(( run_at > now_epoch )) || die "Target time has already passed: $(fmt_epoch "$run_at" '+%F %T')"

# systemd is handed the resolved wall-clock time, so what it fires on is
# exactly what we printed and what date(1) computed.
calendar="$(fmt_epoch "$run_at" '+%Y-%m-%d %H:%M:%S')"
delay=$(( run_at - now_epoch ))

id="$(fmt_epoch "$run_at" '+%Y%m%d-%H%M%S')-${t_pane//[^A-Za-z0-9]/}"
unit="${UNIT_PREFIX}-${id}"

if [[ -n "$opt_keys" ]]; then
  mode="keys"; payload_desc="keys: $opt_keys"
else
  mode="text"; payload_desc="$(print -r -- "$opt_message" | head -1)"
fi

if (( opt_dry_run )); then
  print -r -- "target=$t_kind $t_pane ($t_agent, $t_status)"
  print -r -- "cwd=$t_cwd"
  print -r -- "at=$calendar"
  print -r -- "run_at_epoch=$run_at"
  print -r -- "delay_seconds=$delay"
  print -r -- "mode=$mode"
  print -r -- "payload=$payload_desc"
  print -r -- "id=$id"
  print -r -- "unit=$unit.timer"
  print -r -- "persistent=$opt_persistent"
  print -r -- "if_busy=$opt_if_busy"
  print -r -- "unit_files=$UNIT_DIR/$unit.{service,timer}"
  print -r -- "would_run=$SELF __fire $id"
  exit 0
fi

mkdir -p "$STATE_DIR"
chmod 700 "$STATE_DIR"

# Units from previous runs can linger in "failed" state and leave the user
# manager degraded. The journal keeps the real record; clear the units.
systemctl --user reset-failed "${UNIT_PREFIX}-*" >/dev/null 2>&1 || true

# Drop unit files whose job is finished or was never recorded.
for stale in "$UNIT_DIR/${UNIT_PREFIX}-"*.timer(N); do
  stale_id="${${stale:t:r}#${UNIT_PREFIX}-}"
  [[ -f "$STATE_DIR/$stale_id.json" ]] && continue
  systemctl --user disable "${UNIT_PREFIX}-${stale_id}.timer" >/dev/null 2>&1 || true
  rm -f "$UNIT_DIR/${UNIT_PREFIX}-${stale_id}".{timer,service}
  swept=1
done
(( ${swept:-0} )) && { systemctl --user daemon-reload >/dev/null 2>&1 || true; }

jq -n \
  --arg id "$id" --arg kind "$t_kind" --arg pane_id "$t_pane" \
  --arg agent "$t_agent" --arg cwd "$t_cwd" --arg at "$calendar" \
  --arg mode "$mode" --arg keys "$opt_keys" --arg if_busy "$opt_if_busy" \
  --arg herdr_bin "$HERDR_BIN" --arg unit "$unit" \
  --argjson force "$opt_force" --argjson quiet "$opt_quiet" \
  '{id:$id, kind:$kind, pane_id:$pane_id, agent:$agent, cwd:$cwd, at:$at,
    mode:$mode, keys:$keys, if_busy:$if_busy, force:$force, quiet:$quiet,
    herdr_bin:$herdr_bin, unit:$unit}' \
  > "$STATE_DIR/$id.json"

if [[ "$mode" == text ]]; then
  print -rn -- "$opt_message" > "$STATE_DIR/$id.msg"
fi

persistent_prop="false"
(( opt_persistent )) && persistent_prop="true"

mkdir -p "$UNIT_DIR"

# Unit files live on disk rather than being transient, because transient
# units are written to /run and would be lost on reboot.
cat > "$UNIT_DIR/$unit.service" <<EOF
[Unit]
Description=herdr scheduled send to $t_pane at $calendar

[Service]
Type=oneshot
ExecStart=$SELF __fire $id
EOF

cat > "$UNIT_DIR/$unit.timer" <<EOF
[Unit]
Description=herdr scheduled send timer for $id

[Timer]
OnCalendar=$calendar
AccuracySec=1s
RemainAfterElapse=no
Persistent=$persistent_prop

[Install]
WantedBy=timers.target
EOF

systemctl --user daemon-reload || die "systemctl daemon-reload failed."
systemctl --user enable --now "$unit.timer" >/dev/null 2>&1 \
  || { rm -f "$UNIT_DIR/$unit.service" "$UNIT_DIR/$unit.timer"
       systemctl --user daemon-reload >/dev/null 2>&1 || true
       die "Could not arm $unit.timer."; }

print -r -- "id=$id"
print -r -- "unit=$unit.timer"
print -r -- "target=$t_kind $t_pane ($t_agent)"
print -r -- "at=$calendar"
print -r -- "delay_seconds=$delay"
print -r -- "payload=$payload_desc"
print -r -- ""
print -r -- "cancel with: schedule-send --cancel $id"
