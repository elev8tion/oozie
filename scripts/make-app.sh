#!/bin/sh
# Builds dist/oozie.app — a native Mac app: a Swift/WKWebView shell that
# runs the embedded oozie-server on a free localhost port and hosts the UI
# in its own window. The Go binary is fully self-contained (templates,
# CSS, JS, and migrations are embedded).
set -eu

cd "$(dirname "$0")/.."

APP="dist/oozie.app"
MACOS="$APP/Contents/MacOS"
RESOURCES="$APP/Contents/Resources"

go build -o /tmp/oozie-server ./cmd/app

rm -rf "$APP"
mkdir -p "$MACOS" "$RESOURCES"
mv /tmp/oozie-server "$MACOS/oozie-server"

# Native window shell.
swiftc -O scripts/OozieApp.swift -o "$MACOS/oozie"

# App icon: use the checked-in Apple Intelligence icon; fall back to the
# programmatic one if it's ever missing.
if [ -f assets/icon.png ]; then
  cp assets/icon.png dist/icon.png
else
  swift scripts/make-icon.swift dist/icon.png
fi
ICONSET="dist/AppIcon.iconset"
rm -rf "$ICONSET"; mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" dist/icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z "$d" "$d" dist/icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$RESOURCES/AppIcon.icns"
rm -rf "$ICONSET" dist/icon.png

cat > "$APP/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>oozie</string>
	<key>CFBundleDisplayName</key><string>oozie</string>
	<key>CFBundleExecutable</key><string>oozie</string>
	<key>CFBundleIdentifier</key><string>local.oozie.workspace</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0.0</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF

# Ad-hoc sign the bundle so Finder/Gatekeeper treat it as intact.
command -v codesign >/dev/null && codesign --force --deep -s - "$APP"

echo "built $APP"
echo "install: ditto $APP ~/Applications/oozie.app"
