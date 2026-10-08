// MCP rows migration coverage: proto-built status rows and the detail
// payload must match the shared golden fixtures byte for byte. The list
// rows are flat (the old Go type embedded ServerConfig); the detail nests
// the config under its own key.
package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func mcpFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "mcp", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestMcpRowsProtoMatchFixtures(t *testing.T) {
	transport, tokenRef := "http", "atlassian"
	enabled := true
	full := &consolev1.McpServerStatus{
		Id: "atlassian", Label: "Atlassian", Transport: &transport,
		Url:           strPtr("https://mcp.example"),
		Env:           map[string]string{"REGION": "eu"},
		Auth:          &consolev1.McpAuth{Type: "static", TokenRef: &tokenRef},
		TierOverrides: map[string]string{"read": "write"},
		Enabled:       &enabled,
		CreatedAt:     1700000000000, UpdatedAt: 1700000000001,
		Status: "connected", ToolCount: 2,
		Tools: []*consolev1.McpTool{
			{Name: "t1", Description: strPtr("d1")},
			{Name: "t2"},
		},
	}
	transport2, enabled2 := "stdio", false
	minimal := &consolev1.McpServerStatus{
		Id: "local", Transport: &transport2, Command: strPtr("npx"),
		Args: []string{"-y", "pkg"}, Enabled: &enabled2, Status: "disconnected",
	}
	elements := []json.RawMessage{mustMarshalProto(t, full), mustMarshalProto(t, minimal)}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != mcpFixture(t, "servers.json") {
		t.Fatalf("servers drifted:\n got %s\nwant %s", encoded, mcpFixture(t, "servers.json"))
	}

	// Explicit enabled:false stays present (never omitted): the wire always
	// carried it, and one client defaults a missing key to true.
	if !strings.Contains(string(encoded), `"enabled":false`) {
		t.Fatalf("enabled:false must stay: %s", encoded)
	}
	// int64 timestamps encode as protojson strings (accepted wire change;
	// neither client reads them).
	if !strings.Contains(string(encoded), `"createdAt":"1700000000000"`) {
		t.Fatalf("timestamps must be strings: %s", encoded)
	}
	var _ proto.Message = full
}

func TestMcpDetailProtoMatchesFixture(t *testing.T) {
	transport, tokenRef := "http", "atlassian"
	enabled := true
	config := &consolev1.McpServerConfig{
		Id: "atlassian", Label: "Atlassian", Transport: &transport,
		Url:     strPtr("https://mcp.example"),
		Env:     map[string]string{"REGION": "eu"},
		Auth:    &consolev1.McpAuth{Type: "static", TokenRef: &tokenRef},
		Enabled: &enabled, CreatedAt: 1700000000000, UpdatedAt: 1700000000001,
	}
	configRaw, err := protojson.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	tool := &consolev1.McpTool{Name: "t1", Description: strPtr("d1")}
	toolRaw, err := protojson.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := json.Marshal([]json.RawMessage{toolRaw})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"config": json.RawMessage(configRaw), "tools": json.RawMessage(tools),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != mcpFixture(t, "server_detail.json") {
		t.Fatalf("detail drifted:\n got %s\nwant %s", encoded, mcpFixture(t, "server_detail.json"))
	}
}

// TestMcpListServesProtoRows saves through the desktop's field spellings,
// connects, and reads the list back as proto rows.
func TestMcpListServesProtoRows(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer s3cret"))
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	body := `{"id":"atlassian","name":"Atlassian","transport":"http","url":"` + ts.URL +
		`","auth_type":"static","env":[["REGION","eu"]],"enabled":true,"token":"s3cret"}`
	if code, out := do(t, app, "POST", "/api/mcp/servers", body); code != http.StatusOK || out["success"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	if err := m.Ensure(ctx5(t), "atlassian"); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	code, out := do(t, app, "GET", "/api/mcp/servers", "")
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("list: %d %v", code, out)
	}
	raw, err := json.Marshal(out["data"])
	if err != nil {
		t.Fatal(err)
	}
	var rows []consolev1.McpServerStatus
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("list rows must decode as proto: %v", err)
	}
	if len(rows) != 1 || rows[0].GetLabel() != "Atlassian" {
		t.Fatalf("rows: %+v", rows)
	}
	if rows[0].GetStatus() != "connected" || len(rows[0].GetTools()) == 0 {
		t.Fatalf("live state missing: %+v", rows[0])
	}
	if rows[0].GetEnv()["REGION"] != "eu" {
		t.Fatalf("env must stay an object: %+v", rows[0])
	}
}

func strPtr(s string) *string { return &s }

func mustMarshalProto(t *testing.T, msg proto.Message) json.RawMessage {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
