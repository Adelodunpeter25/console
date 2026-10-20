// Debug route coverage: snapshot shape and notification injection delivery
// for fanout blasts.
//
// The debug routes sit behind the same device-token boundary as every
// other route — they used to have their own bearer middleware, which
// meant two token systems and two revocation states.
package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func debugTestApp(t *testing.T) (*fiber.App, string, *services.NotificationService) {
	t.Helper()
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)

	issuer := auth.NewTokenIssuerWithSecret([]byte("debug-test-secret-32-bytes-!!!"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Bench", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	notifications := services.NewNotificationService()
	app := fiber.New()
	app.Use(routes.RequireDeviceToken(manager))
	routes.RegisterDebugRoutes(app, notifications, func() wt.Stats {
		return wt.Stats{UptimeMillis: 1234, Goroutines: 42, HeapAlloc: 1000, HeapSys: 2000, SessionsTotal: 3, StreamsByKind: map[string]uint64{"CHAT_RUN": 5}}
	})
	return app, paired.Token, notifications
}

func TestDebugStatsRequiresToken(t *testing.T) {
	app, token, _ := debugTestApp(t)
	if code, _ := doTokenRequest(t, app, "GET", "/api/debug/stats", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing token should 401, got %d", code)
	}
	code, out := doTokenRequest(t, app, "GET", "/api/debug/stats", "Bearer "+token)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("stats: %d %v", code, out)
	}
	data, _ := out["data"].(map[string]any)
	for _, key := range []string{"uptimeMillis", "goroutines", "heapAlloc", "heapSys", "sessionsTotal", "streamsByKind"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("snapshot missing %q: %v", key, data)
		}
	}
	if data["sessionsTotal"] != float64(3) {
		t.Fatalf("sessionsTotal: %v", data)
	}
}

func TestDebugNotifyInjectsAndDelivers(t *testing.T) {
	app, token, notifications := debugTestApp(t)
	if code, _ := doTokenRequest(t, app, "POST", "/api/debug/notify", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing token should 401, got %d", code)
	}
	ch := notifications.Subscribe()
	defer notifications.Unsubscribe(ch)
	code, out := doTokenRequest(t, app, "POST", "/api/debug/notify", "Bearer "+token)
	// doTokenRequest sends no body: defaults (bench/test) apply.
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("notify: %d %v", code, out)
	}
	select {
	case event := <-ch:
		if event.GetKind() != "bench" {
			t.Fatalf("kind: %q", event.GetKind())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("injected notification never arrived")
	}
}

func TestWTStatsShapeOnUnstartedServer(t *testing.T) {
	srv := wt.New(wt.Config{Mode: wt.TransportLegacy}, wt.Deps{})
	snap := srv.Stats()
	if snap.UptimeMillis < 0 || snap.Goroutines <= 0 {
		t.Fatalf("process stats: %+v", snap)
	}
	if snap.SessionsTotal != 0 || len(snap.StreamsByKind) != 0 {
		t.Fatalf("fresh server must report zero counters: %+v", snap)
	}
}
