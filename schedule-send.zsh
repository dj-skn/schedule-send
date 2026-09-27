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

# Human-readable name for a resolved row, so every non-picker selection says
# which agent it landed on rather than leaving it to be inferred.
target_summary() {
  local row="$1"
  local kind pane agent st cwd
  kind="$(row_field "$row" 1)"; pane="$(row_field "$row" 2)"
  agent="$(row_field "$row" 3)"; st="$(row_field "$row" 4)"
  cwd="$(pretty_cwd "$(row_field "$row" 5)")"
  if [[ "$kind" == agent ]]; then
    print -r -- "$agent in $pane ($st, $cwd)"
  else
    print -r -- "pane $pane ($cwd)"
  fi
}

choose_target() {
  local -a rows
  rows=("${(@f)$(filter_rows)}")
  [[ -n "${rows[1]:-}" ]] || rows=()

  (( ${#rows} > 0 )) || die "No target matches the given selectors."

  # One match skips the picker, so this note is the only place the choice is
  # stated before the send is armed.
  if (( ${#rows} == 1 )); then
    print -u2 -- "Only one target matched; selecting $(target_summary "${rows[1]}")."
    print -r -- "${rows[1]}"
    return
  fi

  rows=("${(@f)$(sort_rows "${(@)rows}")}")

  if [[ ! -t 0 ]]; then
    if [[ "$opt_pick" == best ]]; then
      print -u2 -- "Multiple targets matched; picking $(target_summary "${rows[1]}")."
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

# An --at value as an epoch, or nothing when date(1) cannot parse it.
#
# A value with no date part means its next occurrence. That includes 12-hour
# forms such as "12:27 AM" and "3pm": handed to date(1) whole they resolve to
# today even once today's has passed. When it has, the time is re-parsed
# against tomorrow's calendar date rather than advanced by 86400, so a DST
# transition between the two days cannot shift the result by an hour.
resolve_at() {
  setopt local_options extended_glob
  local at="$1" now="$2" day epoch zone='' tzp=''

  # Only an ISO timestamp has its T separator swapped out; anywhere else a
  # capital T is part of the text, as in "Tue 3pm".
  [[ "$at" =~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' ]] && at="${at/T/ }"

  # Agents print reset times in forms date(1) rejects as written: an ordinal
  # day ("Sep 27th"), a comma before the time ("Sep 28, 3pm"), and a zone in
  # brackets ("3pm (Europe/Warsaw)"). They are normalised here rather than in
  # the detector, so a time copied out of a notice parses the same as one
  # read from it.
  if [[ "$at" =~ '^(.*[^ ]) *\(([A-Za-z_]+(/[A-Za-z0-9_+-]+)*)\)$' ]]; then
    at="$match[1]"; zone="$match[2]"; tzp="TZ=\"$zone\" "
    # glibc reads a zone it does not know as UTC, without a word.
    [[ -f "${TZDIR:-/usr/share/zoneinfo}/$zone" ]] || { print -r -- ""; return 0 }
  fi
  at="${${at//,/}//(#b)([0-9])(st|nd|rd|th)/$match[1]}"

  if [[ "$at" =~ '^[0-9]{1,2}:[0-9]{2}(:[0-9]{2})?$' \
     || "$at" =~ '^[0-9]{1,2}(:[0-9]{2}(:[0-9]{2})?)? ?[AaPp][Mm]$' ]]; then
    # "Today" is the zone's own date, which differs from ours near midnight.
    if [[ -n "$zone" ]]; then
      day="$(TZ="$zone" date -d "@$now" +%F)"
    else
      day="$(fmt_epoch "$now" +%F)"
    fi
    epoch="$(parse_epoch "$tzp$day $at")"
    if [[ -n "$epoch" ]] && (( epoch <= now )); then
      day="$(date -d "$day +1 day" +%F)"
      epoch="$(parse_epoch "$tzp$day $at")"
    fi
  else
    epoch="$(parse_epoch "$tzp$at")"
  fi
  print -r -- "$epoch"
}

# A delay in seconds as "2h 5m", rounded up to the minute.
fmt_delay() {
  local m=$(( ($1 + 59) / 60 ))
  if   (( m >= 1440 )); then print -r -- "$(( m / 1440 ))d $(( m % 1440 / 60 ))h"
  elif (( m >= 60 ));   then print -r -- "$(( m / 60 ))h $(( m % 60 ))m"
  else                       print -r -- "${m}m"
  fi
}

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

# How long --if-busy wait gives a working agent to settle.
readonly BUSY_WAIT_MS=900000

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

# Herdr confirms a prompt by observing a lifecycle state change, but that
# observation is not a receipt. Codex has no state hook: herdr scrapes its
# state from the OSC title and its idle rule matches any non-empty title, so
# Codex's state flaps on its own and a prompt whose Enter the TUI dropped still
# satisfies --wait. Herdr records the mirror-image bug for Copilot, where a
# background pane swallowed the keystrokes entirely until prompting was made to
# focus the pane first. Either way the text is left in the composer as an
# unsubmitted draft while the job reports success.
#
# So the send is gated on evidence instead, and the composer outranks the
# lifecycle: a draft still on screen disproves any status the agent claims.
# Only that evidence authorises a keystroke, which is what keeps a retry from
# answering a dialog or landing a second copy of the message.

readonly NEEDLE_LEN=24
readonly NEEDLE_MIN=8

typeset -g W_RECOVER=keys W_GRACE=4
typeset -g OBS_STATUS=unknown OBS_SEQ=-1
typeset -g OBS_TAIL='' OBS_BODY='' OBS_BODY_OK=0
typeset -ga OBS_RAW=()
typeset -g VERDICT=unknown SAW_DRAFT=0 RECOVERY=''

# Alphanumerics only, folded to lower case. A composer wraps the text it holds,
# and wrapping injects padding, newlines, and gutter glyphs into the middle of
# it; reducing both sides to this form makes the comparison survive all three.
squash() { local s="${1//[^[:alnum:]]/}"; print -r -- "${s:l}" }

# A long prompt scrolls inside the composer and only its end stays on screen,
# so the end of the message is the part we can reliably look for. Too short a
# needle would match noise, so it is dropped rather than trusted.
msg_needle() {
  local squashed
  squashed="$(squash "$1")"
  (( ${#squashed} >= NEEDLE_MIN )) || { print -r -- ""; return 0 }
  print -r -- "${squashed[-NEEDLE_LEN,-1]}"
}

# A horizontal rule, tested by removing the glyphs that draw one rather than
# through a bracket expression: the worker runs under systemd with no locale,
# where a byte-wise matcher would union the leading UTF-8 byte of every glyph
# and fire on anything that happens to share it.
is_rule() {
  local t="${1//[ 	]/}"
  [[ -n "$t" ]] || return 1
  t="${t//─/}"; t="${t//━/}"; t="${t//═/}"; t="${t//╌/}"
  t="${t//-/}"; t="${t//=/}"; t="${t//_/}"
  [[ -z "$t" ]]
}

# A line opening with a prompt marker: an agent's composer, or a prompt it
# echoed into its transcript. Matched as strings, not a bracket expression,
# for the same locale reason as is_rule.
is_prompt_line() {
  local t="${${1//[ 	]/}#('│'|'|')}"
  [[ "$t" == ('›'|'❯'|'>')* ]]
}

# One screen read feeds both detectors, which need different slices of it.
# OBS_TAIL is the last composer marker down to the bottom of the screen, the
# domain the needle is searched in. OBS_BODY is that marker up to the rule that
# closes the composer, the domain emptiness is judged in — the cut matters,
# because below that rule Claude draws a status footer whose context, cost, and
# elapsed-time counters move on their own and would make every composer look
# occupied. OBS_BODY_OK stays 0 when the region cannot be delimited, which
# switches the emptiness detector off rather than letting it guess.
read_composer() {
  OBS_TAIL=''; OBS_BODY=''; OBS_BODY_OK=0; OBS_RAW=()
  local -a lines
  lines=("${(@f)$("$HERDR_BIN" agent read "$1" --source visible --lines 40 2>/dev/null)}")
  (( ${#lines} )) || return 0

  local i start=0 end=0
  for (( i = 1; i <= ${#lines}; i++ )); do
    is_prompt_line "${lines[$i]}" && start=$i
  done
  (( start )) || return 0
  OBS_TAIL="$(squash "${(j::)${(@)lines[$start,-1]}}")"
  OBS_RAW=("${(@)lines[$start,-1]}")

  for (( i = start + 1; i <= ${#lines}; i++ )); do
    if is_rule "${lines[$i]}"; then end=$i; break; fi
  done
  (( end )) || return 0
  OBS_BODY="$(squash "${(j::)${(@)lines[$start,end-1]}}")"
  OBS_RAW=("${(@)lines[$start,end-1]}")
  OBS_BODY_OK=1
}

# state_change_seq is stamped on an agent when its own status transitions, so
# comparing it against a baseline is edge-triggered: it still reports a turn
# that started and finished between two polls, which comparing the status
# itself would miss. It degrades to -1 if herdr ever stops reporting it, and
# the status comparison then carries the gate alone.
read_agent_state() {
  OBS_STATUS=unknown; OBS_SEQ=-1
  local json pair
  json="$("$HERDR_BIN" agent get "$1" 2>/dev/null)" || return 0
  pair="$(print -r -- "$json" | jq -r '
    [(.result.agent.agent_status // "unknown"),
     (.result.agent.state_change_seq // -1)] | @tsv' 2>/dev/null)" || return 0
  [[ -n "$pair" ]] || return 0
  OBS_STATUS="$(row_field "$pair" 1)"
  OBS_SEQ="$(row_field "$pair" 2)"
  [[ "$OBS_SEQ" == (-|)<-> ]] || OBS_SEQ=-1
}

# sent | unsent | unknown, in that order of authority. The composer is read
# first on purpose: whatever the lifecycle says, a draft we can still see means
# the Enter did not land.
submission_verdict() {
  local pane="$1" needle="$2" seq0="$3" gate="$4"

  read_composer "$pane"
  if [[ -n "$needle" && "$OBS_TAIL" == *"$needle"* ]]; then
    SAW_DRAFT=1; VERDICT=unsent; return 0
  fi
  if (( gate && OBS_BODY_OK )) && [[ -n "$OBS_BODY" ]]; then
    SAW_DRAFT=1; VERDICT=unsent; return 0
  fi

  read_agent_state "$pane"
  if [[ "$OBS_STATUS" == working || "$OBS_STATUS" == blocked ]]; then
    VERDICT=sent; return 0
  fi
  if (( seq0 >= 0 && OBS_SEQ > seq0 )); then
    VERDICT=sent; return 0
  fi
  # A draft that was on screen a moment ago and is not there now went
  # somewhere, and the only way out of a composer is a submit.
  if (( SAW_DRAFT )); then
    VERDICT=sent; return 0
  fi
  VERDICT=unknown
}

# Polls until the verdict is decisive or the grace window closes, backing off
# so a genuinely dropped Enter costs a handful of reads rather than twenty.
await_verdict() {
  local pane="$1" needle="$2" seq0="$3" gate="$4"
  local deadline=$(( SECONDS + W_GRACE )) polls=0
  while true; do
    submission_verdict "$pane" "$needle" "$seq0" "$gate"
    [[ "$VERDICT" == sent ]] && return 0
    (( SECONDS >= deadline )) && return 0
    polls=$(( polls + 1 ))
    if   (( polls < 3 )); then sleep 0.25
    elif (( polls < 6 )); then sleep 0.5
    else                       sleep 1
    fi
  done
}

focused_agent() {
  "$HERDR_BIN" agent list 2>/dev/null \
    | jq -r 'first(.result.agents[] | select(.focused) | .pane_id) // empty' 2>/dev/null \
    || true
}

# The focus rung is a repair, not a context switch, so focus goes back to
# whoever held it. Only an agent can be refocused this way; when a plain pane
# held focus there is nothing to hand it back to.
restore_focus() {
  local prev="$1" pane="$2"
  [[ -n "$prev" && "$prev" != "$pane" ]] || return 0
  "$HERDR_BIN" agent focus "$prev" >/dev/null 2>&1 || true
}

# The escalation ladder, entered only on a verdict of unsent. It presses Enter
# and never re-sends the text: if the first Enter did land after all, a second
# copy of the message would be far worse than an Enter into an empty composer,
# which every supported TUI ignores.
# 0 recovered, 1 exhausted, 2 a dialog opened and the ladder must not touch it.
recover_submission() {
  local pane="$1" needle="$2" seq0="$3" gate="$4"
  local -a rungs
  case "$W_RECOVER" in
    none)  return 1 ;;
    focus) rungs=(keys focus) ;;
    *)     rungs=(keys) ;;
  esac

  local rung prev_focus=''
  for rung in "${(@)rungs}"; do
    read_agent_state "$pane"
    if [[ "$OBS_STATUS" == blocked ]]; then
      restore_focus "$prev_focus" "$pane"
      return 2
    fi

    if [[ "$rung" == focus ]]; then
      # Herdr's own notes record background panes swallowing input until the
      # pane was focused, so focusing is the escalation rather than a louder
      # keypress. It also marks a "done" agent as seen, which reads as idle.
      print -u2 -- "Retried Enter did not land in $pane; focusing the pane and trying again."
      prev_focus="$(focused_agent)"
      "$HERDR_BIN" agent focus "$pane" >/dev/null 2>&1 || true
      RECOVERY="focused, Enter retried"
    else
      print -u2 -- "Enter was dropped by the agent in $pane; retrying the submit."
      RECOVERY="Enter retried"
    fi

    if ! "$HERDR_BIN" agent send-keys "$pane" enter >/dev/null 2>&1; then
      print -u2 -- "Could not send Enter to $pane."
      continue
    fi

    await_verdict "$pane" "$needle" "$seq0" "$gate"
    if [[ "$VERDICT" == sent ]]; then
      restore_focus "$prev_focus" "$pane"
      return 0
    fi
  done

  restore_focus "$prev_focus" "$pane"
  return 1
}

# The draft the user will find in the composer, quoted into the failure so the
# journal and the notification say what is stuck rather than only that
# something is.
composer_excerpt() {
  read_composer "$1"
  local i out=''
  for (( i = 1; i <= ${#OBS_RAW}; i++ )); do
    [[ -n "${OBS_RAW[$i]//[ 	]/}" ]] || continue
    out="$out ${OBS_RAW[$i]}"
  done
  print -r -- "$out" | tr -s ' 	' ' ' | cut -c1-160
}

# A usage-limited agent takes a prompt like any other: the text leaves the
# composer and the lifecycle moves, so the gate reports a submit. The agent
# then answers with a limit notice instead of a turn, and the job would report
# a success that never happened. So after the submit the transcript below our
# message is watched for that notice.
#
# Both Codex and Claude echo a submitted prompt into the transcript, so the
# last copy of the message's end marks where this send's answer begins, and a
# notice from an earlier limit sits above it. The anchor has no minimum length
# the way the composer needle does: a short one can only land later in the
# transcript than our message, which misses a notice but never invents one.
readonly LIMIT_WATCH=10

typeset -ga TR_LINES=() TR_SQ=()
typeset -g LIMIT_EXCERPT=''

# The pane's recent transcript into TR_LINES, and each line squashed into
# TR_SQ. Claude draws on the alternate screen, whose rows never reach the host
# scrollback, and herdr can recover that history only from an idle agent — so
# a recent read can come back empty, and the visible screen, which still holds
# the latest exchange, stands in for it.
read_transcript() {
  local out
  out="$("$HERDR_BIN" agent read "$1" --source recent-unwrapped --lines "$2" 2>/dev/null)"
  [[ -n "${out//[[:space:]]/}" ]] \
    || out="$("$HERDR_BIN" agent read "$1" --source visible --lines "$2" 2>/dev/null)"
  TR_LINES=("${(@f)out}")

  # squash() inlined: this runs for every line on every poll.
  TR_SQ=()
  local i s
  for (( i = 1; i <= ${#TR_LINES}; i++ )); do
    s="${TR_LINES[$i]//[^[:alnum:]]/}"
    TR_SQ[i]="${s:l}"
  done
}

# A squashed line that opens a usage-limit notice: Codex's "■ You’ve hit your
# usage limit" or "Usage limit reached", Claude's "You've hit your session
# limit". Squashing strips the glyph and the curly apostrophe, so the match
# does not depend on the worker's missing locale.
is_limit_line() {
  [[ "$1" == youvehityour*limit* || "$1" == usagelimitreached* ]]
}

# The notice opening at transcript line $1, joined back into one line. Codex
# wraps it itself, even in the middle of a date, so the lines after it are
# taken too — up to the blank line or rule that ends it, or a line indented
# well past it, such as the tip Codex centres underneath.
notice_text() {
  local i="$1" j out="${TR_LINES[$1]}" lead
  lead="${TR_LINES[i]%%[^ ]*}"
  local indent=${#lead}
  for (( j = i + 1; j <= i + 5 && j <= ${#TR_LINES}; j++ )); do
    [[ -n "${TR_LINES[j]//[ 	]/}" ]] || break
    is_rule "${TR_LINES[j]}" && break
    lead="${TR_LINES[j]%%[^ ]*}"
    (( ${#lead} < indent + 8 )) || break
    out="$out ${TR_LINES[j]}"
  done
  print -r -- "$out" | tr -s ' 	' ' '
}

# 0 when a transcript line after the message's end opens a limit notice.
limit_notice_after() {
  local pane="$1" anchor="$2"
  LIMIT_EXCERPT=''
  read_transcript "$pane" 80
  local i all="${(j::)TR_SQ}"
  [[ -n "$anchor" && "$all" == *"$anchor"* ]] || return 1

  # The line holding the last character of the last copy of the anchor.
  local end=$(( ${#${all%"$anchor"*}} + ${#anchor} )) acc=0
  for (( i = 1; i <= ${#TR_SQ}; i++ )); do
    acc=$(( acc + ${#TR_SQ[i]} ))
    (( acc >= end )) && break
  done

  for (( i = i + 1; i <= ${#TR_SQ}; i++ )); do
    is_limit_line "${TR_SQ[i]}" || continue
    LIMIT_EXCERPT="$(notice_text "$i" | cut -c1-200)"
    return 0
  done
  return 1
}

# Holds the job open for LIMIT_WATCH seconds after the submit, which is long
# enough for the rejected request to come back. 1 when a notice showed up.
await_limit_notice() {
  local pane="$1" anchor="$2"
  local deadline=$(( SECONDS + LIMIT_WATCH ))
  [[ -n "$anchor" ]] || return 0
  while (( SECONDS < deadline )); do
    sleep 1
    limit_notice_after "$pane" "$anchor" && return 1
  done
  return 0
}

deliver_agent_text() {
  local pane="$1" msg="$2"
  local needle seq0 gate=0
  needle="$(msg_needle "$msg")"

  # Baseline taken before anything is typed: the lifecycle counter to compare
  # against, and whether the composer was empty. If it already held a draft of
  # the user's own we cannot tell their text from ours, so the emptiness
  # detector stays off and only the needle speaks.
  read_agent_state "$pane"
  seq0=$OBS_SEQ
  read_composer "$pane"
  (( OBS_BODY_OK )) && [[ -z "$OBS_BODY" ]] && gate=1
  SAW_DRAFT=0
  RECOVERY=''

  if [[ -z "$needle" ]] && (( ! gate )); then
    print -u2 -- "Note: neither submit detector applies to $pane; the send cannot be verified."
  fi

  # --wait is deliberately not used. It reports a lifecycle change that a
  # self-flapping agent produces on its own, and when it sees none it fails
  # with agent_prompt_stalled — which is the very case this gate exists to
  # recover from, and treating it as fatal here would make the ladder
  # unreachable. It also holds the job open for the whole turn, so the check
  # would run minutes after the moment it could still fix anything.
  local prompt_err detail
  if ! prompt_err="$("$HERDR_BIN" agent prompt "$pane" "$msg" 2>&1)"; then
    detail="$(print -r -- "$prompt_err" | jq -r '.error.message // empty' 2>/dev/null || true)"
    [[ -n "$detail" ]] || detail="$(print -r -- "$prompt_err" | head -1)"
    worker_fail "agent prompt to $pane was refused: ${detail:-unknown error}"
  fi

  await_verdict "$pane" "$needle" "$seq0" "$gate"

  local note='' rc=0
  case "$VERDICT" in
    sent) ;;
    unknown)
      # Nothing in the composer and nothing moved. Pressing Enter here would
      # be the blind keystroke this gate replaced, so the send is reported as
      # delivered but unverified instead of being guessed either way.
      print -u2 -- "Delivered to $pane but the submit is unverified: no draft in its composer and no state change."
      note=' (unverified)'
      ;;
    *)
      recover_submission "$pane" "$needle" "$seq0" "$gate" || rc=$?
      case $rc in
        0) note=" ($RECOVERY)" ;;
        2) worker_fail "Agent in $pane opened a dialog with the text unsent; it is left in the composer." ;;
        *) worker_fail "Text is still unsent in $pane's composer: $(composer_excerpt "$pane")" ;;
      esac
      ;;
  esac

  local anchor
  anchor="$(squash "$msg")"
  await_limit_notice "$pane" "${anchor[-NEEDLE_LEN,-1]}" \
    || worker_fail "Agent in $pane is still usage-limited; the prompt was not acted on: $LIMIT_EXCERPT"
  print -r -- "prompted agent in $pane$note"
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

  # Defaulted rather than read straight, so a timer armed by an older version
  # of this script still fires: its state file predates both keys.
  W_RECOVER="$(jq -r '.recover // "keys"' "$state")"
  W_GRACE="$(jq -r '.grace // 4' "$state")"
  [[ "$W_GRACE" == <-> ]] || W_GRACE=4

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
      # A wait that timed out is not a settled agent, and prompting it anyway
      # would type into a turn that is still running. Only "working" is
      # refused: an unknown status is Codex's flapping title, which the submit
      # gate is there to see through.
      "$HERDR_BIN" agent wait "$pane" --timeout "$BUSY_WAIT_MS" >/dev/null 2>&1 || true
      read_agent_state "$pane"
      [[ "$OBS_STATUS" != working ]] \
        || worker_fail "Agent in $pane was still working after $(( BUSY_WAIT_MS / 60000 )) minutes; send aborted."
    fi
    deliver_agent_text "$pane" "$msg"
  else
    "$HERDR_BIN" pane run "$pane" "$msg" >/dev/null \
      || worker_fail "pane run in $pane failed."
    print -r -- "ran text in $pane"
  fi

  worker_cleanup
}

# ----------------------------------------------------- usage-limit timing ---

# What a run without --at sends when no payload was given: resuming after the
# limit is what a detected time is for.
readonly RESUME_MESSAGE=resume

# A notice names its reset to the minute, and the reset lands somewhere inside
# that minute — a notice reading 12:27 AM was for a reset at 00:27:52 — so the
# first moment certainly past it is one minute on.
readonly LIMIT_MARGIN=60

typeset -g LN_TIME='' LN_QUOTE='' LIMIT_SOURCE='' LIMIT_RESET=''

# The pane's current usage-limit notice. LN_TIME is the reset time it names, as
# printed, or empty when it names none ("try again later"); LN_QUOTE is the
# phrase to show back. 1 when there is no notice, or only one the agent has
# since moved past.
limit_notice() {
  LN_TIME=''; LN_QUOTE=''
  read_transcript "$1" 120
  local i last=0 prompts=0
  for (( i = 1; i <= ${#TR_SQ}; i++ )); do
    is_limit_line "${TR_SQ[i]}" && last=$i
  done
  (( last )) || return 1

  # A prompt echoed below the newest notice went through without drawing
  # another, so that limit is over. The last prompt line is the composer.
  for (( i = last + 1; i <= ${#TR_LINES}; i++ )); do
    is_prompt_line "${TR_LINES[i]}" && prompts=$(( prompts + 1 ))
  done
  (( prompts < 2 )) || return 1

  # Codex: "try again at Sep 27th, 2026 12:27 AM". Claude: "resets 3pm",
  # "resets Sep 28, 3:30pm", either optionally followed by "(Europe/Warsaw)".
  local text re
  text="$(notice_text "$last")"
  re='([Tt]ry again at|resets at|resets) (([A-Z][a-z]{2} [0-9]{1,2}(st|nd|rd|th)?,? ([0-9]{4},? )?)?[0-9]{1,2}(:[0-9]{2})? ?[AaPp][Mm]( \([A-Za-z_]+(/[A-Za-z0-9_+-]+)*\))?)'
  if [[ "$text" =~ "$re" ]]; then
    LN_TIME="$match[2]"; LN_QUOTE="$MATCH"
  else
    LN_QUOTE="$(print -r -- "$text" | cut -c1-160)"
  fi
}

# The reset for a target: its own notice first, then another agent of the same
# kind, since the limit belongs to the account rather than the pane. Sets
# LIMIT_SOURCE to the pane the notice came from and LIMIT_RESET to its epoch,
# left empty when the notice names no time date(1) can read. A notice whose
# reset already passed is old scrollback and is skipped. 1 when none is found.
find_limit() {
  local pane="$1" agent="$2" now="$3" row p epoch
  local -a panes=("$pane")
  for row in "${(@)ROWS}"; do
    [[ "$(row_field "$row" 1)" == agent && "$(row_field "$row" 3)" == "$agent" ]] || continue
    p="$(row_field "$row" 2)"
    [[ "$p" != "$pane" && "$p" != "${HERDR_PANE_ID:-}" ]] && panes+=("$p")
  done

  LIMIT_SOURCE=''; LIMIT_RESET=''
  for p in "${(@)panes}"; do
    limit_notice "$p" || continue
    epoch=''
    [[ -n "$LN_TIME" ]] && epoch="$(resolve_at "$LN_TIME" "$now")"
    [[ -n "$epoch" ]] && (( epoch <= now )) && continue
    LIMIT_SOURCE="$p"; LIMIT_RESET="$epoch"
    return 0
  done
  return 1
}

# What was found, for the confirmation prompt or --auto's note on stderr.
limit_report() {
  local now="$1" what
  if [[ -z "$LIMIT_SOURCE" ]]; then
    print -r -- "No usage-limit notice found for $(target_summary "$target")."
    return 0
  fi
  if [[ -n "$opt_keys" ]]; then
    what="keys: $opt_keys"
  else
    what="\"${opt_message%%$'\n'*}\""
  fi

  if [[ "$LIMIT_SOURCE" == "$t_pane" ]]; then
    print -r -- "Usage limit in $t_agent $t_pane:"
  else
    print -r -- "Usage limit in $t_agent $LIMIT_SOURCE (per account; $t_pane shows none):"
  fi
  print -r -- "  $LN_QUOTE"
  if [[ -n "$LIMIT_RESET" ]]; then
    print -r -- "Resets $(fmt_epoch "$LIMIT_RESET" '+%a %H:%M') (in $(fmt_delay $(( LIMIT_RESET - now )))); sending $what $(( LIMIT_MARGIN / 60 )) min after."
  else
    print -r -- "It names no reset time that can be read; sending $what at the time given."
  fi
}

# Asks on the terminal when to send, after showing the report, offering $1 (an
# epoch, or empty for no default). Enter takes the default. Anything typed goes
# through resolve_at, is shown back, and becomes the new default, so a
# correction is confirmed the same way the detected time is.
confirm_at() {
  local def="$1" now="$2" report="$3" tty_fd input epoch
  exec {tty_fd}<>/dev/tty || die "Could not open /dev/tty to confirm the send time."
  print -r -- "" >&$tty_fd
  print -r -- "$report" >&$tty_fd
  print -r -- "" >&$tty_fd

  while true; do
    if [[ -n "$def" ]]; then
      printf 'at [%s]: ' "$(fmt_epoch "$def" '+%F %H:%M')" >&$tty_fd
    else
      printf 'at: ' >&$tty_fd
    fi
    IFS= read -r input <&$tty_fd || die "No send time given."
    if [[ -z "$input" ]]; then
      [[ -n "$def" ]] && break
      continue
    fi
    epoch="$(resolve_at "$input" "$now")"
    if [[ -z "$epoch" ]]; then
      print -r -- "Could not parse '$input'. Try 00:45, 12:45 AM, Sep 27 1am, or +30 min." >&$tty_fd
    elif (( epoch <= now )); then
      print -r -- "$(fmt_epoch "$epoch" '+%F %H:%M') has already passed." >&$tty_fd
    else
      print -r -- "  → $(fmt_epoch "$epoch" '+%a %F %H:%M:%S') (in $(fmt_delay $(( epoch - now ))))" >&$tty_fd
      def="$epoch"
    fi
  done
  exec {tty_fd}>&-
  print -r -- "$def"
}

# The send time for a run without --at: the target's usage-limit reset plus
# LIMIT_MARGIN, confirmed on the terminal, or taken as found under --auto.
detect_at() {
  local now="$1" proposed='' report
  LIMIT_SOURCE=''; LIMIT_RESET=''
  if [[ "$t_kind" == agent ]]; then
    find_limit "$t_pane" "$t_agent" "$now" || true
  fi
  [[ -n "$LIMIT_RESET" ]] && proposed=$(( LIMIT_RESET + LIMIT_MARGIN ))
  report="$(limit_report "$now")"

  if (( opt_auto )); then
    [[ -n "$proposed" ]] \
      || die "$report"$'\n'"--auto needs a notice that names its reset time; pass --at instead."
    print -u2 -- "$report"
    print -r -- "$proposed"
    return 0
  fi
  confirm_at "$proposed" "$now" "$report"
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
opt_recover="keys"; opt_grace=4
opt_panes_only=0; opt_allow_self=0; opt_force=0
opt_quiet=0; opt_dry_run=0; opt_persistent=0; opt_auto=0
have_message=0; message_defaulted=0

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
    --auto)         opt_auto=1; shift ;;
    --message)      opt_message="$(flag_value "$1" "${2-}")"; have_message=1; shift 2 ;;
    --message-file) opt_message_file="$(flag_value "$1" "${2-}")"; have_message=1; shift 2 ;;
    --keys)         opt_keys="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --pick)         opt_pick="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --if-busy)      opt_if_busy="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --recover)      opt_recover="$(flag_value "$1" "${2-}")"; shift 2 ;;
    --submit-grace) opt_grace="$(flag_value "$1" "${2-}")"; shift 2 ;;
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
[[ "$opt_recover" == (none|keys|focus) ]] \
  || die "--recover must be 'none', 'keys', or 'focus'."
[[ "$opt_grace" == <-> ]] && (( opt_grace >= 1 && opt_grace <= 60 )) \
  || die "--submit-grace must be a whole number of seconds from 1 to 60."

[[ -z "$opt_at" ]] || (( ! opt_auto )) || die "Use either --at or --auto, not both."

# Without --at the send time comes from the target's usage-limit notice, and a
# detected time is only taken unseen when --auto says so.
detect=0
[[ -n "$opt_at" ]] || detect=1
if (( detect && ! opt_auto )) && [[ ! -t 0 ]]; then
  die "No --at, and no terminal to confirm a detected time on. Pass --at, or --auto to take the detected time unconfirmed."
fi

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
if (( detect && ! have_message )) && [[ -z "$opt_keys" ]]; then
  opt_message="$RESUME_MESSAGE"
  message_defaulted=1
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
if (( message_defaulted )) && [[ "$t_kind" != agent ]]; then
  die "Pane $t_pane is a plain shell, where the default '$RESUME_MESSAGE' would run as a command. Pass --message."
fi
if [[ "$t_kind" == agent && "$t_status" == blocked && -z "$opt_keys" ]]; then
  print -u2 -- "Warning: agent in $t_pane is blocked on a prompt right now."
fi

now_epoch="${HERDR_SCHEDULE_NOW_EPOCH:-$(date +%s)}"
[[ "$now_epoch" == <-> ]] || die "HERDR_SCHEDULE_NOW_EPOCH must be an integer epoch."

if (( detect )); then
  run_at="$(detect_at "$now_epoch")"
  # A confirmation can sit unanswered for a while, so "already passed" and the
  # delay are judged from the moment it was answered.
  [[ -n "${HERDR_SCHEDULE_NOW_EPOCH:-}" ]] || now_epoch="$(date +%s)"
else
  run_at="$(resolve_at "$opt_at" "$now_epoch")"
  [[ -n "$run_at" ]] \
    || die "Could not parse --at '$opt_at'. Use HH:MM[:SS], H[:MM]am/pm, or YYYY-MM-DD HH:MM:SS."
fi

(( run_at > now_epoch )) || die "Target time has already passed: $(fmt_epoch "$run_at" '+%F %T')"

# systemd is handed the resolved wall-clock time, so what it fires on is
# exactly what we printed and what date(1) computed.
calendar="$(fmt_epoch "$run_at" '+%Y-%m-%d %H:%M:%S')"
delay=$(( run_at - now_epoch ))

id="$(fmt_epoch "$run_at" '+%Y%m%d-%H%M%S')-${t_pane//[^A-Za-z0-9]/}"
unit="${UNIT_PREFIX}-${id}"

# The id is the fire time plus the pane, so a second send to the same pane at
# the same second would overwrite the first job's state and units while its
# timer is still armed.
[[ ! -e "$STATE_DIR/$id.json" ]] \
  || die "A send to $t_pane at $calendar is already scheduled ($id). Cancel it with --cancel $id, or pick another time."

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
  print -r -- "recover=$opt_recover"
  print -r -- "submit_grace=$opt_grace"
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
  --arg recover "$opt_recover" --argjson grace "$opt_grace" \
  --arg herdr_bin "$HERDR_BIN" --arg unit "$unit" \
  --argjson force "$opt_force" --argjson quiet "$opt_quiet" \
  '{id:$id, kind:$kind, pane_id:$pane_id, agent:$agent, cwd:$cwd, at:$at,
    mode:$mode, keys:$keys, if_busy:$if_busy, force:$force, quiet:$quiet,
    recover:$recover, grace:$grace,
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
print -r -- "target=$t_kind $t_pane ($t_agent, $t_status)"
print -r -- "at=$calendar"
print -r -- "delay_seconds=$delay"
print -r -- "payload=$payload_desc"
print -r -- ""
print -r -- "cancel with: schedule-send --cancel $id"
