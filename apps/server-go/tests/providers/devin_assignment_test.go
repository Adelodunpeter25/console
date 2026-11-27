// Devin router pinning: one routed model per conversation until the
// assignment JWT is about to expire, so the prompt cache is not split.
package tests

import (
	"fmt"
	"github.com/MoeForge/moego/base64"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/devin"
)

func jwtExpiringAt(exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return "h." + payload + ".s"
}

func TestDevinAssignmentIsPinnedUntilItExpires(t *testing.T) {
	var pins devin.PinnedAssignments
	now := time.Now()
	pins.Put("conv", "adaptive", "model-a", jwtExpiringAt(now.Add(time.Hour)), now)

	if uid, ok := pins.Get("conv", "adaptive", now.Add(30*time.Minute)); !ok || uid != "model-a" {
		t.Fatalf("pinned = %q, %v; want model-a", uid, ok)
	}
	if _, ok := pins.Get("conv", "adaptive", now.Add(59*time.Minute+30*time.Second)); ok {
		t.Fatal("an assignment inside its expiry margin must be re-routed")
	}
	if _, ok := pins.Get("other", "adaptive", now); ok {
		t.Fatal("pins are per conversation")
	}
	if _, ok := pins.Get("conv", "other-router", now); ok {
		t.Fatal("a different router uid must not reuse the pin")
	}
}

func TestDevinAssignmentWithoutReadableExpiryUsesFallbackTTL(t *testing.T) {
	var pins devin.PinnedAssignments
	now := time.Now()
	pins.Put("conv", "adaptive", "model-a", "opaque", now)
	if _, ok := pins.Get("conv", "adaptive", now.Add(5*time.Minute)); !ok {
		t.Fatal("expected the pin within the fallback window")
	}
	if _, ok := pins.Get("conv", "adaptive", now.Add(11*time.Minute)); ok {
		t.Fatal("expected the pin to lapse after the fallback window")
	}
}
