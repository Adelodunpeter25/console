// The macOS bundle is the whole trick behind TCC attribution: the driver reads
// Accessibility and Screen Recording off the bundle identifier in Info.plist,
// not off the executable. These cover the parts that can be checked without
// actually granting anything.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/commands"
)

func TestBundlePlistCarriesIdentityAndUsageStrings(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("bundle is macOS-only")
	}
	plist := commands.BundlePlistForTest("console")

	// The identifier is what TCC keys the grant rows to; without it macOS has
	// nothing stable to attach a permission to.
	if !strings.Contains(plist, "<string>sh.console.computeruse</string>") {
		t.Error("plist has no bundle identifier")
	}
	// macOS refuses to show a permission prompt for an app with no declared
	// purpose, so both usage strings are load-bearing, not decoration.
	for _, key := range []string{"NSAccessibilityUsageDescription", "NSScreenCaptureUsageDescription"} {
		if !strings.Contains(plist, key) {
			t.Errorf("plist is missing %s", key)
		}
	}
	// The executable name must match the binary actually copied in.
	if !strings.Contains(plist, "<string>console</string>") {
		t.Error("plist does not name the executable")
	}
	if !strings.Contains(plist, "<string>APPL</string>") {
		t.Error("plist is not marked as an application bundle")
	}
}

func TestBundleBuildProducesSignedDirectoryLayout(t *testing.T) {
	if _, err := os.Stat("/usr/bin/codesign"); err != nil {
		t.Skip("codesign is unavailable, so this is macOS-only")
	}
	dir := filepath.Join(t.TempDir(), "Test Bundle.app")

	// The real builder copies os.Executable(), which during a test is the test
	// binary. That is fine: the layout and signing are what is under test.
	if err := commands.BuildBundle(dir, "-"); err != nil {
		t.Fatalf("build bundle: %v", err)
	}

	for _, rel := range []string{
		"Contents/Info.plist",
		"Contents/MacOS",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("bundle is missing %s: %v", rel, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(dir, "Contents", "MacOS"))
	if err != nil {
		t.Fatalf("read MacOS dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("bundle contains no executable")
	}

	// Verification is the point: an unsigned bundle cannot hold a TCC grant.
	out, err := runCodesignVerify(dir)
	if err != nil {
		t.Fatalf("codesign --verify: %v\n%s", err, out)
	}
}
func runCodesignVerify(path string) (string, error) {
	out, err := exec.Command("codesign", "--verify", "--verbose=2", path).CombinedOutput()
	return string(out), err
}
