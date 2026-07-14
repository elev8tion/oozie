package build

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	writeTestPNG(t, filepath.Join(dir, "icon.png"))

	appPath, err := SwiftBuilder{}.Build(dir, "Hello Oozie", "http://127.0.0.1:8080/api/beacon/hello-oozie")
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
	// The beacon shim wraps the real binary.
	shim, _ := os.ReadFile(exe)
	if !strings.Contains(string(shim), "api/beacon/hello-oozie") {
		t.Error("launcher shim missing beacon URL")
	}
	if info, err := os.Stat(exe + "-bin"); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("real binary missing beside shim: %v", err)
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

	if _, err := os.Stat(filepath.Join(appPath, "Contents", "Resources", "AppIcon.icns")); err != nil {
		t.Errorf("icon.png was not converted to AppIcon.icns: %v", err)
	}
	plist, _ := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
	if !strings.Contains(string(plist), "CFBundleIconFile") {
		t.Error("Info.plist missing CFBundleIconFile")
	}
	if _, err := exec.LookPath("codesign"); err == nil {
		if err := exec.Command("codesign", "--verify", appPath).Run(); err != nil {
			t.Errorf("bundle signature does not verify: %v", err)
		}
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for x := 0; x < 64; x++ {
		for y := 0; y < 64; y++ {
			img.Set(x, y, color.RGBA{R: 120, G: 160, B: 255, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFailsWithoutPackage(t *testing.T) {
	_, err := SwiftBuilder{}.Build(t.TempDir(), "Nope", "")
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
