#!/bin/sh
# Builds dist/oozie.app — a double-clickable Mac app that starts the oozie
# server and opens it in your default browser. The Go binary is fully
# self-contained (templates, CSS, JS, and migrations are embedded).
set -eu

cd "$(dirname "$0")/.."

APP="dist/oozie.app"
MACOS="$APP/Contents/MacOS"

go build -o /tmp/oozie-server ./cmd/app

rm -rf "$APP"
mkdir -p "$MACOS"
mv /tmp/oozie-server "$MACOS/oozie-server"

cat > "$MACOS/oozie" <<'EOF'
#!/bin/sh
DIR="$(cd "$(dirname "$0")" && pwd)"
export OOZIE_OPEN_BROWSER=1
exec "$DIR/oozie-server"
EOF
chmod 755 "$MACOS/oozie"

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
	<key>NSHighResolutionCapable</key><true/>
	<key>LSUIElement</key><true/>
</dict>
</plist>
EOF

# Ad-hoc sign the bundle so Finder/Gatekeeper treat it as intact.
command -v codesign >/dev/null && codesign --force --deep -s - "$APP"

echo "built $APP"
echo "install: ditto $APP ~/Applications/oozie.app"
