// Package build turns a project working directory into a runnable macOS
// .app bundle. The supported project shape is a Swift package with an
// executable target (Package.swift); a prebuilt .app found in the project
// is used as-is.
package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AppBuilder produces a .app bundle for a project and returns its path.
// beaconURL, when non-empty, is baked into a launcher shim that pings it
// (fire-and-forget, localhost) on every launch so oozie's store can show
// which apps are actually alive.
type AppBuilder interface {
	Build(workdir, appName, beaconURL string) (string, error)
}

// SwiftBuilder builds Swift packages with `swift build -c release` and
// wraps the produced executable in a minimal .app bundle under dist/.
type SwiftBuilder struct {
	Timeout time.Duration // zero means 10 minutes
}

func (b SwiftBuilder) Build(workdir, appName, beaconURL string) (string, error) {
	appName = sanitizeAppName(appName)

	if _, err := os.Stat(filepath.Join(workdir, "Package.swift")); err != nil {
		if prebuilt := findPrebuiltApp(workdir); prebuilt != "" {
			return prebuilt, nil
		}
		return "", fmt.Errorf("no Package.swift (or prebuilt .app) found in %s — ask the agent to scaffold a Swift executable package first", workdir)
	}

	timeout := b.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	buildCmd := exec.CommandContext(ctx, "swift", "build", "-c", "release")
	buildCmd.Dir = workdir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("swift build failed: %s", tail(string(out), 2000))
	}

	binPathCmd := exec.CommandContext(ctx, "swift", "build", "-c", "release", "--show-bin-path")
	binPathCmd.Dir = workdir
	binPathOut, err := binPathCmd.Output()
	if err != nil {
		return "", fmt.Errorf("locate build products: %w", err)
	}
	binPath := strings.TrimSpace(string(binPathOut))

	executable, err := findExecutable(binPath, appName)
	if err != nil {
		return "", err
	}

	return assembleBundle(workdir, appName, executable, beaconURL)
}

// assembleBundle writes dist/<AppName>.app with the executable, an app
// icon when the project provides one, and a minimal Info.plist, then
// ad-hoc signs the bundle. Returns the bundle's absolute path.
func assembleBundle(workdir, appName, executable, beaconURL string) (string, error) {
	bundle := filepath.Join(workdir, "dist", appName+".app")
	macos := filepath.Join(bundle, "Contents", "MacOS")
	if err := os.RemoveAll(bundle); err != nil {
		return "", err
	}
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return "", err
	}

	// With a beacon URL the bundle's main "executable" is a two-line shim
	// that pings oozie (localhost, 1s cap, silent when oozie is closed)
	// and execs the real binary, renamed <name>-bin.
	execName := filepath.Base(executable)
	if beaconURL != "" {
		if err := copyFile(executable, filepath.Join(macos, execName+"-bin"), 0o755); err != nil {
			return "", fmt.Errorf("copy executable: %w", err)
		}
		shim := fmt.Sprintf("#!/bin/sh\n# oozie liveness beacon — localhost only; a lost ping is fine.\ncurl -m 1 -s %q >/dev/null 2>&1 &\nexec \"$(dirname \"$0\")/%s-bin\" \"$@\"\n", beaconURL, execName)
		if err := os.WriteFile(filepath.Join(macos, execName), []byte(shim), 0o755); err != nil {
			return "", fmt.Errorf("write launcher shim: %w", err)
		}
	} else if err := copyFile(executable, filepath.Join(macos, execName), 0o755); err != nil {
		return "", fmt.Errorf("copy executable: %w", err)
	}

	iconEntry := ""
	if installIcon(workdir, bundle) {
		iconEntry = "\t<key>CFBundleIconFile</key><string>AppIcon</string>\n"
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>%s</string>
	<key>CFBundleDisplayName</key><string>%s</string>
	<key>CFBundleExecutable</key><string>%s</string>
	<key>CFBundleIdentifier</key><string>local.oozie.%s</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0.0</string>
	<key>NSHighResolutionCapable</key><true/>
%s</dict>
</plist>
`, appName, appName, execName, bundleSlug(appName), iconEntry)
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return "", err
	}

	// Sign so Finder and Gatekeeper treat the bundle as intact. With
	// OOZIE_SIGN_IDENTITY set to a keychain code-signing identity, updates
	// keep the same signer, so TCC permission grants (Screen Recording,
	// Accessibility) survive republishes; ad-hoc ("-") re-identifies the
	// app on every build and macOS forgets its permissions.
	// Failure is non-fatal: the executable is already linker-signed.
	if _, err := exec.LookPath("codesign"); err == nil {
		identity := signingIdentity()
		if out, err := exec.Command("codesign", "--force", "--deep", "-s", identity, bundle).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "codesign (non-fatal): %s\n", strings.TrimSpace(string(out)))
			if identity != "-" {
				// Named identity failed (missing cert?) — fall back to ad-hoc
				// so the bundle still verifies.
				_ = exec.Command("codesign", "--force", "--deep", "-s", "-", bundle).Run()
			}
		}
	}

	abs, err := filepath.Abs(bundle)
	if err != nil {
		return bundle, nil
	}
	return abs, nil
}

// installIcon looks for an icon the project provides — icon.icns, or a
// PNG (icon.png / AppIcon.png / Icon.png) converted via sips + iconutil —
// and writes Contents/Resources/AppIcon.icns. Returns true on success.
func installIcon(workdir, bundle string) bool {
	resources := filepath.Join(bundle, "Contents", "Resources")
	target := filepath.Join(resources, "AppIcon.icns")

	if src := filepath.Join(workdir, "icon.icns"); fileExists(src) {
		if os.MkdirAll(resources, 0o755) == nil && copyFile(src, target, 0o644) == nil {
			return true
		}
		return false
	}

	var png string
	for _, name := range []string{"icon.png", "AppIcon.png", "Icon.png"} {
		if p := filepath.Join(workdir, name); fileExists(p) {
			png = p
			break
		}
	}
	if png == "" {
		return false
	}
	if _, err := exec.LookPath("sips"); err != nil {
		return false
	}
	if _, err := exec.LookPath("iconutil"); err != nil {
		return false
	}

	iconset, err := os.MkdirTemp("", "oozie-iconset-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(iconset)
	set := filepath.Join(iconset, "AppIcon.iconset")
	if err := os.Mkdir(set, 0o755); err != nil {
		return false
	}
	for _, size := range []int{16, 32, 128, 256, 512} {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			px := size * scale
			out := filepath.Join(set, fmt.Sprintf("icon_%dx%d%s.png", size, size, suffix))
			if err := exec.Command("sips", "-z", fmt.Sprint(px), fmt.Sprint(px), png, "--out", out).Run(); err != nil {
				return false
			}
		}
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return false
	}
	return exec.Command("iconutil", "-c", "icns", set, "-o", target).Run() == nil
}

var (
	identityOnce   sync.Once
	cachedIdentity string
)

// signingIdentity picks the signer for published bundles:
// OOZIE_SIGN_IDENTITY, else the first valid code-signing identity in the
// keychain, else ad-hoc. A stable identity means TCC permission grants
// survive republishes.
func signingIdentity() string {
	identityOnce.Do(func() {
		cachedIdentity = "-"
		if env := os.Getenv("OOZIE_SIGN_IDENTITY"); env != "" {
			cachedIdentity = env
			return
		}
		out, err := exec.Command("security", "find-identity", "-p", "codesigning", "-v").Output()
		if err != nil {
			return
		}
		// Lines look like: 1) <40-hex-sha1> "Apple Development: Name (TEAM)"
		for _, line := range strings.Split(string(out), "\n") {
			if i := strings.Index(line, `"`); i >= 0 {
				if j := strings.LastIndex(line, `"`); j > i {
					cachedIdentity = line[i+1 : j]
					return
				}
			}
		}
	})
	return cachedIdentity
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// findExecutable picks the built executable from the Swift bin path,
// preferring a name match with the app.
func findExecutable(binPath, appName string) (string, error) {
	entries, err := os.ReadDir(binPath)
	if err != nil {
		return "", fmt.Errorf("read build products: %w", err)
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.Contains(name, ".") { // skips .dylib, .a, .swiftmodule, .product, .json…
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			continue
		}
		candidates = append(candidates, filepath.Join(binPath, name))
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("swift build produced no executable — the package needs an executable target")
	}
	want := strings.ToLower(strings.ReplaceAll(appName, " ", ""))
	for _, c := range candidates {
		if strings.ToLower(filepath.Base(c)) == want {
			return c, nil
		}
	}
	return candidates[0], nil
}

// findPrebuiltApp returns a .app bundle already present in the project
// (root or dist/), for projects the agent built with xcodebuild directly.
func findPrebuiltApp(workdir string) string {
	for _, dir := range []string{workdir, filepath.Join(workdir, "dist")} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.app"))
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && info.IsDir() {
				abs, err := filepath.Abs(m)
				if err == nil {
					return abs
				}
				return m
			}
		}
	}
	return ""
}

func sanitizeAppName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "App"
	}
	return out
}

// Slug is the app's stable beacon/bundle identity, derived from its name.
func Slug(name string) string {
	return strings.ToLower(strings.ReplaceAll(sanitizeAppName(name), " ", "-"))
}

func bundleSlug(name string) string { return Slug(name) }

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
