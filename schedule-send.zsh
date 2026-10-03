#!/bin/zsh
#
# schedule-send is a Go program now: see cmd/schedule-send and tools/install.
# This file stays because timers armed before the port fire
# "<this file> __fire <id>", and the binary reads their state files as they are.

bin=${0:A:h}/bin/schedule-send
if [[ ! -x $bin ]]; then
  print -u2 -- "schedule-send is not built: run tools/install in ${0:A:h}."
  exit 1
fi
exec $bin "$@"
