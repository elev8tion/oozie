package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSwiftBuilderProducesAppBundle builds a minimal Swift executable
// package and checks that a valid .app bundle comes out. Skipped when the
// Swift toolchain is unavailable.
func TestSwiftBuilderProducesAppBundle(t *testing.T) {
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("swift toolchain not installed")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Package.swift"), `// swift-tools-version:5.9
import PackageDescription
let package = Package(
    name: "HelloOozie",
    platforms: [.macOS(.v13)],
    targets: [.executableTarget(name: "HelloOozie", path: "Sources/HelloOozie")]
)
`)
	writeFile(t, filepath.Join(dir, "Sources", "HelloOozie", "main.swift"), `print("hello from oozie")`)

	appPath, err := SwiftBuilder{}.Build(dir, "Hello Oozie")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if filepath.Base(appPath) != "Hello Oozie.app" {
		t.Errorf("bundle name = %s, want Hello Oozie.app", filepath.Base(appPath))
	}
	exe := filepath.Join(appPath, "Contents", "MacOS", "HelloOozie")
	if info, err := os.Stat(exe); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("bundle executable missing or not executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appPath, "Contents", "Info.plist")); err != nil {
		t.Fatalf("Info.plist missing: %v", err)
	}
	out, err := exec.Command(exe).Output()
	if err != nil {
		t.Fatalf("run bundled executable: %v", err)
	}
	if string(out) != "hello from oozie\n" {
		t.Errorf("executable output = %q", out)
	}
}

func TestBuildFailsWithoutPackage(t *testing.T) {
	_, err := SwiftBuilder{}.Build(t.TempDir(), "Nope")
	if err == nil {
		t.Fatal("expected error for empty project")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
