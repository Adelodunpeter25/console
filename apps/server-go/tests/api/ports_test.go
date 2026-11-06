// Ports migration coverage: proto-built payloads must match the shared
// golden fixtures byte-for-byte, forward keeps its number-or-string
// leniency, and validation is preserved. The tunnel endpoint is a byte
// pipe and is not covered here.
package tests

import (
	"encoding/json"
	"fmt"
	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
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
	registry := terminalsvc.NewPortRegistry()
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

// A forwarder listening on the port it forwards makes every tunnel dial back
// into itself, multiplying connections. The cap must admit a healthy number
// of tunnels and refuse the flood with a distinct close code.
func TestTunnelLoopGuard(t *testing.T) {
	registry := terminalsvc.NewPortRegistry()
	t.Cleanup(registry.CloseAll)
	app := fiber.New()
	routes.RegisterPortRoutes(app, registry)

	// Upstream that holds every connection open, so tunnels keep their slots.
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	accepted := make(chan net.Conn, 32)
	go func() {
		for {
			conn, err := up.Accept()
			if err != nil {
				return
			}
			accepted <- conn
		}
	}()
	t.Cleanup(func() {
		for {
			select {
			case conn := <-accepted:
				conn.Close()
			default:
				return
			}
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	port := up.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("ws://%s/api/ports/%d/tunnel", ln.Addr(), port)

	// maxTunnelsPerPort (routes) is 8: admit 8, refuse the 9th.
	var held []*websocket.Conn
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	for i := 0; i < 8; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("tunnel %d must be admitted: %v", i+1, err)
		}
		held = append(held, conn)
		// The server dials the upstream only after taking its slot, so an
		// arriving connection proves the tunnel is holding it.
		select {
		case <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatalf("tunnel %d never reached the upstream", i+1)
		}
	}

	loopConn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if loopConn != nil {
		loopConn.Close()
	}
	if err == nil {
		t.Fatal("flood tunnel must be refused")
	}
	if resp == nil || resp.StatusCode != 503 {
		t.Fatalf("flood tunnel must refuse 503, got %v", resp)
	}

	// A freed slot admits a new tunnel again.
	held[0].Close()
	held = held[1:]
	var admitted *websocket.Conn
	for attempt := 0; attempt < 50; attempt++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			admitted = conn
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if admitted == nil {
		t.Fatal("released slot must admit a new tunnel")
	}
	admitted.Close()
}
