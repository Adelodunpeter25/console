// Ports migration coverage: proto-built payloads must match the shared
// golden fixtures byte-for-byte, forward keeps its number-or-string
// leniency, and validation is preserved. The tunnel endpoint is a byte
// pipe and is not covered here.
package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func portFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "ports", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestPortProtoMatchesFixture(t *testing.T) {
	single, err := protojson.Marshal(&consolev1.ForwardedPort{
		Port: 5173, Url: "http://localhost:45173",
	})
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, single) != portFixture(t, "port.json") {
		t.Fatalf("port bytes drifted:\n got %s\nwant %s", compactJSON(t, single), portFixture(t, "port.json"))
	}

	pid := "p1"
	arr, err := json.Marshal([]json.RawMessage{
		mustCompact(t, mustProto(t, &consolev1.ForwardedPort{Port: 3000, Url: "http://localhost:45000"})),
		mustCompact(t, mustProto(t, &consolev1.ForwardedPort{
			Port: 5173, Url: "http://localhost:45173", ProjectId: &pid,
		})),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Fixture lists ports ascending, matching the sorted snapshot order.
	if strings.TrimSpace(string(arr)) != portFixture(t, "list.json") {
		t.Fatalf("list bytes drifted:\n got %s\nwant %s", arr, portFixture(t, "list.json"))
	}

	del, err := protojson.Marshal(&consolev1.UnforwardPortResponse{Port: 5173})
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, del) != portFixture(t, "unforward.json") {
		t.Fatalf("delete bytes drifted:\n got %s\nwant %s", compactJSON(t, del), portFixture(t, "unforward.json"))
	}
}

func mustProto(t *testing.T, msg *consolev1.ForwardedPort) []byte {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustCompact(t *testing.T, raw []byte) json.RawMessage {
	t.Helper()
	var buf strings.Builder
	_ = buf
	return json.RawMessage(compactJSON(t, raw))
}

func TestPortRoutesForward(t *testing.T) {
	registry := services.NewPortRegistry()
	t.Cleanup(registry.CloseAll)
	app := fiber.New()
	routes.RegisterPortRoutes(app, registry)

	// A real listener so Forward's liveness probe passes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	post := func(body string) (int, map[string]any) {
		req := httptest.NewRequest("POST", "/api/ports/forward", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var envelope struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("envelope: %s", raw)
		}
		var data map[string]any
		if envelope.Success {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				t.Fatalf("data: %s", envelope.Data)
			}
		}
		return resp.StatusCode, data
	}

	// Number form.
	if code, data := post(fmt.Sprintf(`{"port":%d}`, port)); code != 200 || int(data["port"].(float64)) != port {
		t.Fatalf("forward number: %d %v", code, data)
	}
	// String form stays accepted (legacy leniency).
	if code, _ := post(fmt.Sprintf(`{"port":"%d"}`, port)); code != 200 {
		t.Fatalf("forward string must stay accepted: %d", code)
	}
	// Missing body and dead ports are still 400.
	if code, _ := post(`{}`); code != 400 {
		t.Fatalf("missing port must 400: %d", code)
	}
	if code, _ := post(`{"port":1}`); code != 400 {
		t.Fatalf("dead port must 400: %d", code)
	}

	// List shows the entry; delete removes it, twice deletes nothing.
	req := httptest.NewRequest("GET", "/api/ports/", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), fmt.Sprintf(`"port":%d`, port)) {
		t.Fatalf("list missing entry: %s", raw)
	}
	del := httptest.NewRequest("DELETE", fmt.Sprintf("/api/ports/%d", port), nil)
	if resp, err := app.Test(del, 10000); err != nil || resp.StatusCode != 200 {
		t.Fatalf("delete must 200: %v", resp)
	}
	if resp, err := app.Test(del, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("second delete must 400: %v", resp)
	}
}
