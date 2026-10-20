package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func TestPairingGrantIsSingleUseAndProtectsRoutes(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("pairing-test-secret-32-bytes-long"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	routes.RegisterPairingRoutes(app, manager)
	app.Use(routes.RequireDeviceToken(manager))
	app.Get("/private", func(c *fiber.Ctx) error { return c.SendString("ok") })

	unauthorized, _ := app.Test(httptest.NewRequest("GET", "/private", nil))
	if unauthorized.StatusCode != 401 {
		t.Fatalf("private route status = %d", unauthorized.StatusCode)
	}

	body := `{"grant":"` + grant + `","deviceName":"Desktop","platform":"desktop"}`
	pairReq := httptest.NewRequest("POST", "/api/auth/pair", newBody(body))
	pairReq.Header.Set("Content-Type", "application/json")
	response, err := app.Test(pairReq)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("pair status = %d", response.StatusCode)
	}
	// The payload is protojson, so it decodes with the shared wire type.
	paired := decodePairData(t, response.Body)
	if paired.GetToken() == "" {
		t.Fatal("pair response omitted token")
	}
	if paired.GetDevice().GetId() == "" {
		t.Fatal("pair response omitted the device")
	}
	if paired.GetDevice().GetName() != "Desktop" {
		t.Fatalf("device name = %q", paired.GetDevice().GetName())
	}
	if paired.GetDevice().GetPlatform() != "desktop" {
		t.Fatalf("device platform = %q", paired.GetDevice().GetPlatform())
	}
	if paired.GetExpiresAt() == 0 {
		t.Fatal("pair response omitted the expiry")
	}

	authorizedReq := httptest.NewRequest("GET", "/private", nil)
	authorizedReq.Header.Set("Authorization", "Bearer "+paired.GetToken())
	authorized, _ := app.Test(authorizedReq)
	if authorized.StatusCode != 200 {
		t.Fatalf("authorized route status = %d", authorized.StatusCode)
	}

	reuseReq := httptest.NewRequest("POST", "/api/auth/pair", newBody(body))
	reuseReq.Header.Set("Content-Type", "application/json")
	reused, _ := app.Test(reuseReq)
	if reused.StatusCode != 401 {
		t.Fatalf("reused grant status = %d", reused.StatusCode)
	}
}

// The device list is protojson too, and its timestamps are optional: a
// device that has never been seen must not report an epoch timestamp.
func TestDeviceListUsesTheSharedWireType(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("device-list-secret-32-bytes-long"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	routes.RegisterPairingRoutes(app, manager)
	app.Use(routes.RequireDeviceToken(manager))
	routes.RegisterDeviceRoutes(app, manager)

	req := httptest.NewRequest("GET", "/api/auth/devices", nil)
	req.Header.Set("Authorization", "Bearer "+paired.Token)
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("device list status = %d", response.StatusCode)
	}
	list := decodeDeviceList(t, response.Body)
	if len(list.GetDevices()) != 1 {
		t.Fatalf("expected one device, got %d", len(list.GetDevices()))
	}
	device := list.GetDevices()[0]
	if device.GetId() != paired.ID {
		t.Fatalf("device id = %q want %q", device.GetId(), paired.ID)
	}
	// The request that fetched the list was itself authenticated, so the
	// device has been seen — the list must say so.
	if device.GetLastSeenAt() == 0 {
		t.Fatal("a device that just made an authenticated request must report a last-seen time")
	}
}

// A device that has never made a request reports no last-seen time at
// all: "never seen" must stay distinguishable from "seen at the epoch".
func TestDeviceThatHasNeverConnectedHasNoLastSeen(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("never-seen-secret-32-bytes-long"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	// The manager is asked for the list directly, so this device's token
	// is never presented and it is never seen.
	devices, err := manager.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device, got %d", len(devices))
	}
	if devices[0].LastSeenAt != nil {
		t.Fatalf("a device that has never connected must not report a timestamp: %v", devices[0].LastSeenAt)
	}
	_ = paired
}

func decodePairData(t *testing.T, body io.Reader) *consolev1.PairDeviceResponse {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	out := &consolev1.PairDeviceResponse{}
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal(envelope.Data, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func decodeDeviceList(t *testing.T, body io.Reader) *consolev1.ServerDevicesResponse {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	out := &consolev1.ServerDevicesResponse{}
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal(envelope.Data, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func newBody(value string) *strings.Reader { return strings.NewReader(value) }
