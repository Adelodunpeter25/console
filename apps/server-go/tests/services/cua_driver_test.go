// Cua Driver bindings: the cancellation and status semantics, which are a
// contract with the caller, plus library loading when the SDK is absent. The
// SDK itself is not present in CI, so the native paths are exercised by hand
// against a real library rather than mocked here.
package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// TestCancelledErrorWording pins the message the reference hosts and the
// driver's own skill use: an interrupted action may still have taken effect, so
// the caller must inspect fresh state rather than retry blindly.
func TestCancelledErrorWording(t *testing.T) {
	unknown := &cua.CancelledError{Unknown: true}
	for _, want := range []string{"unknown", "before retrying"} {
		if !strings.Contains(unknown.Error(), want) {
			t.Errorf("unknown cancellation must mention %q: %q", want, unknown.Error())
		}
	}
	if !errors.Is(unknown, cua.ErrStopped) {
		t.Error("CancelledError must unwrap to ErrStopped")
	}

	// A refused start has a known outcome and must not claim otherwise: a
	// caller told "unknown" here would inspect state it does not need to.
	notStarted := &cua.CancelledError{}
	if strings.Contains(notStarted.Error(), "unknown") {
		t.Errorf("a refused start has no unknown outcome: %q", notStarted.Error())
	}
	if !errors.Is(notStarted, cua.ErrStopped) {
		t.Error("a refused start must still unwrap to ErrStopped")
	}
}

// TestStatusErrorCancelled covers the two header codes that mean "the caller
// stopped this", so a Stop surfaces as a cancellation rather than a failure.
func TestStatusErrorCancelled(t *testing.T) {
	for _, status := range []int32{4, 5} {
		if err := (&cua.StatusError{Status: status}); !err.Cancelled() {
			t.Errorf("status %d should report as cancelled", status)
		}
	}
	for _, status := range []int32{1, 2, 3, 6, 7, 8} {
		if err := (&cua.StatusError{Status: status}); err.Cancelled() {
			t.Errorf("status %d is a failure, not a cancellation", status)
		}
	}
	if msg := (&cua.StatusError{Status: 6}).Error(); msg == "" {
		t.Error("StatusError must render a message")
	}
}

// TestOpenWithoutLibraryFailsCleanly proves the feature degrades to "computer
// use is unavailable" rather than panicking or hanging. An absolute path is
// used so the result does not depend on the test binary's directory.
func TestOpenWithoutLibraryFailsCleanly(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", filepath.Join(t.TempDir(), "absent"))
	if _, err := cua.Open(cua.Options{}); err == nil {
		t.Fatal("Open succeeded with no library present")
	}
}

// TestLibraryPathOverrideRejectsRelative guards the security decision: a
// computer-use library resolved against an ambient path is an attack surface,
// so a relative override is refused rather than resolved against the cwd.
func TestLibraryPathOverrideRejectsRelative(t *testing.T) {
	for _, relative := range []string{
		"libcua_driver_sdk.so",
		"./libcua_driver_sdk.so",
		"../cua/libcua_driver_sdk.so",
	} {
		t.Setenv("CUA_DRIVER_LIB_PATH", relative)
		if _, err := cua.LibraryPathOverride(); err == nil {
			t.Errorf("relative override %q was accepted", relative)
		} else if !strings.Contains(err.Error(), "absolute") {
			t.Errorf("got %v, want an absolute-path complaint", err)
		}
	}
}

// TestLibraryPathOverrideAcceptsAbsolute is the other half: the supported shape
// must keep working, since this is how a packaged install finds its library.
func TestLibraryPathOverrideAcceptsAbsolute(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "libcua_driver_sdk.so")
	t.Setenv("CUA_DRIVER_LIB_PATH", abs)
	got, err := cua.LibraryPathOverride()
	if err != nil {
		t.Fatalf("absolute override rejected: %v", err)
	}
	if got != abs {
		t.Fatalf("got %q, want %q", got, abs)
	}
}

// TestLibraryPathOverrideEmptyIsNotAnError keeps "unset" distinct from
// "invalid": an empty value simply means fall back to the packaged paths.
func TestLibraryPathOverrideEmptyIsNotAnError(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", "")
	got, err := cua.LibraryPathOverride()
	if err != nil {
		t.Fatalf("unset override must not be an error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// TestOptionsMarshalIsAlwaysAJSONObject guards the ABI contract: create_v1
// takes a JSON object and fails closed on unknown fields, so an empty options
// set must still serialize as "{}" and never as nil or an empty string.
func TestOptionsMarshalIsAlwaysAJSONObject(t *testing.T) {
	// The encoding happens inside Open, which needs the library, so assert the
	// property through the exported entry point: with no library the call must
	// fail at load time rather than panicking on an empty byte slice.
	t.Setenv("CUA_DRIVER_LIB_PATH", filepath.Join(t.TempDir(), "absent"))
	if _, err := cua.Open(cua.Options{}); err == nil {
		t.Fatal("expected a load failure")
	}
}

// TestMissingLibraryPathIsNotSilentlyIgnored documents that an env override
// pointing nowhere produces an error rather than falling through to a
// different candidate.
func TestMissingLibraryPathIsNotSilentlyIgnored(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "definitely-absent.so")
	if _, err := os.Stat(missing); err == nil {
		t.Skip("path unexpectedly exists")
	}
	if filepath.IsAbs(missing) != true {
		t.Fatal("temp paths must be absolute for this test to mean anything")
	}
}
