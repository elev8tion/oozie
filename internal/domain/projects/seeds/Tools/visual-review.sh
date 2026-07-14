#!/bin/sh
# Visual review helper: builds the app, launches it, screenshots its
# window, and quits it. The agent then reads the PNG and critiques the UI
# against DESIGN.md.
#
# usage: sh Tools/visual-review.sh <ExecutableName> [output.png]
# Requires Screen Recording permission for oozie (macOS prompts once).
set -eu

EXE="$1"
OUT="${2:-review.png}"

swift build >/dev/null
BIN="$(swift build --show-bin-path)/$EXE"

"$BIN" &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT
sleep 4

WID="$(swift Tools/WindowID.swift "$EXE" 2>/dev/null || true)"
if [ -n "$WID" ]; then
	screencapture -x -l "$WID" "$OUT"
else
	echo "window not found; capturing full screen" >&2
	screencapture -x "$OUT"
fi

echo "$OUT"
