#!/bin/sh
# Install schedule-send on Linux: download the release binary for this
# machine, check it against the published SHA-256, and put it at
# ~/.local/bin/schedule-send, the one place a release binary arms jobs from.
#
#   curl -fsSL https://raw.githubusercontent.com/dj-skn/schedule-send/main/install.sh | sh
#
# Run it again to upgrade. SCHEDULE_SEND_VERSION picks a release instead of
# the latest. SCHEDULE_SEND_DOWNLOAD replaces where the files are fetched
# from, for testing.
set -eu

repo=https://github.com/dj-skn/schedule-send

die() {
  echo "schedule-send install: $*" >&2
  exit 1
}

# Everything is in a function, so a download cut short runs nothing.
main() {
  case "$(uname -s)" in
    Linux) ;;
    Darwin) die "on macOS, install with Homebrew: brew install dj-skn/tap/schedule-send" ;;
    *) die "$(uname -s) is not supported; schedule-send runs on Linux and macOS." ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no release is built for $(uname -m); build from source: $repo" ;;
  esac
  [ -n "${HOME:-}" ] || die "HOME is not set."
  for tool in curl tar sha256sum; do
    command -v "$tool" > /dev/null || die "$tool is required."
  done

  if [ -n "${SCHEDULE_SEND_DOWNLOAD:-}" ]; then
    base=$SCHEDULE_SEND_DOWNLOAD
  elif [ -n "${SCHEDULE_SEND_VERSION:-}" ]; then
    base=$repo/releases/download/v${SCHEDULE_SEND_VERSION#v}
  else
    base=$repo/releases/latest/download
  fi
  asset=schedule-send-linux-$arch.tar.gz
  dest=$HOME/.local/bin/schedule-send

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "could not download $base/$asset"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || die "could not download $base/checksums.txt"
  # Exactly one line may name the archive; a file that lists it twice, or not
  # at all, proves nothing.
  grep " $asset\$" "$tmp/checksums.txt" > "$tmp/expected" || true
  [ "$(wc -l < "$tmp/expected")" -eq 1 ] || die "checksums.txt does not name $asset exactly once."
  (cd "$tmp" && sha256sum -c expected > /dev/null 2>&1) || die "$asset does not match its published SHA-256; nothing was installed."
  tar -xzf "$tmp/$asset" -C "$tmp" schedule-send || die "$asset could not be unpacked."

  mkdir -p "$HOME/.local/bin"
  [ ! -L "$dest" ] || echo "Replacing the link at $dest, which pointed to $(readlink "$dest")."
  # Written beside the destination and renamed, so a job that fires during
  # an upgrade runs either the old binary or the new one, never half of one.
  install -m 755 "$tmp/schedule-send" "$dest.new.$$"
  mv -f "$dest.new.$$" "$dest"

  echo "Installed $("$dest" --version) at $dest"
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) echo "Add $HOME/.local/bin to your PATH to run it as schedule-send." ;;
  esac
  command -v herdr > /dev/null || echo "Herdr is not on your PATH; schedule-send needs it: https://herdr.dev"
  command -v systemctl > /dev/null || echo "systemctl was not found; schedule-send needs systemd user timers."
  echo "Jobs fire while you are logged in. To keep them through a logout: loginctl enable-linger \"\$USER\""
}

main "$@"
