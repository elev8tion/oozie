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
	"time"
)

// AppBuilder produces a .app bundle for a project and returns its path.
type AppBuilder interface {
	Build(workdir, appName string) (string, error)
}

// SwiftBuilder builds Swift packages with `swift build -c release` and
// wraps the produced executable in a minimal .app bundle under dist/.
type SwiftBuilder struct {
	Timeout time.Duration // zero means 10 minutes
}

func (b SwiftBuilder) Build(workdir, appName string) (string, error) {
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

	return assembleBundle(workdir, appName, executable)
}

// assembleBundle writes dist/<AppName>.app with the executable and a
// minimal Info.plist, and returns the bundle's absolute path.
func assembleBundle(workdir, appName, executable string) (string, error) {
	bundle := filepath.Join(workdir, "dist", appName+".app")
	macos := filepath.Join(bundle, "Contents", "MacOS")
	if err := os.RemoveAll(bundle); err != nil {
		return "", err
	}
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return "", err
	}

	execName := filepath.Base(executable)
	if err := copyFile(executable, filepath.Join(macos, execName), 0o755); err != nil {
		return "", fmt.Errorf("copy executable: %w", err)
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
</dict>
</plist>
`, appName, appName, execName, bundleSlug(appName))
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(bundle)
	if err != nil {
		return bundle, nil
	}
	return abs, nil
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

func bundleSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

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
