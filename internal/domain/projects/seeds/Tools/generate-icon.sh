#!/bin/sh
# Generates an app icon with on-device Apple Intelligence (Image Playground).
#
# usage: sh Tools/generate-icon.sh "<prompt>" [icon.png] [style]
#   style: illustration (default) | animation | sketch
#
# Good prompts: "flat minimal app icon of <subject> on a rounded square
# <color> background, no text". The generator app flashes briefly in the
# Dock — that's required by the API. On failure the reason prints to stderr
# (e.g. Apple Intelligence disabled); fall back to drawing an icon with
# AppKit instead.
set -eu

PROMPT="$1"
OUT="${2:-icon.png}"
STYLE="${3:-illustration}"

DIR="$(cd "$(dirname "$0")" && pwd)"
APP="$DIR/.icongen/IconGen.app"

if [ ! -x "$APP/Contents/MacOS/IconGen" ]; then
	mkdir -p "$APP/Contents/MacOS"
	swiftc -O "$DIR/IconGen.swift" -o "$APP/Contents/MacOS/IconGen"
	cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>IconGen</string>
	<key>CFBundleExecutable</key><string>IconGen</string>
	<key>CFBundleIdentifier</key><string>local.oozie.icongen</string>
	<key>CFBundlePackageType</key><string>APPL</string>
</dict>
</plist>
PLIST
	codesign --force --deep -s - "$APP" 2>/dev/null || true
fi

case "$OUT" in
	/*) ABS="$OUT" ;;
	*) ABS="$(pwd)/$OUT" ;;
esac
rm -f "$ABS" "$ABS.err"

open -W "$APP" --args "$PROMPT" "$ABS" "$STYLE"

if [ -f "$ABS.err" ]; then
	cat "$ABS.err" >&2
	rm -f "$ABS.err"
	exit 1
fi
[ -f "$ABS" ] || { echo "icon generation produced no file" >&2; exit 1; }
echo "$ABS"
