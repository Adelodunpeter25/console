// Assist migration coverage: the two /api/assist payloads must match the
// shared golden fixtures. The route tests for command discoverability live in
// assist_test.go.
package tests

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func assistFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "assist", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func checkAssistFixture(t *testing.T, name string, msg proto.Message) {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != assistFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), assistFixture(t, name))
	}
}

func checkAssistListFixture(t *testing.T, name string, items []proto.Message) {
	t.Helper()
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protojson.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, json.RawMessage(compactJSON(t, raw)))
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != assistFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, encoded, assistFixture(t, name))
	}
}

// The commands payload keeps its camelCase keys and builtin flag; protojson
// omits builtin:false, which is the correct shape for a discovered skill.
func TestAssistCommandsFixture(t *testing.T) {
	checkAssistListFixture(t, "commands.json", []proto.Message{
		&consolev1.SlashCommandInfo{Name: "init", Description: "Generate a console.toml with project run scripts for the Run tab", Builtin: true},
		&consolev1.SlashCommandInfo{Name: "computer-use", Description: "Drive the computer: read app windows and operate them with keyboard and mouse", Builtin: true},
		&consolev1.SlashCommandInfo{Name: "find-skills", Description: "Helps users discover and install agent skills when they ask them to", Builtin: false},
	})
}

// The search envelope round-trips its items unchanged and echoes the raw
// query — including the empty one a bare "@" produces.
func TestAssistSearchFixture(t *testing.T) {
	checkAssistFixture(t, "search.json", &consolev1.AssistFileSearchResponse{
		Root:  "/p",
		Query: "DiffView",
		Items: []*consolev1.FileSearchResult{
			{RelativePath: "src/DiffView.kt", AbsolutePath: "/p/src/DiffView.kt", Score: 0.95},
			{RelativePath: "src/dir", AbsolutePath: "/p/src/dir", IsDir: true, Score: 0.5},
		},
	})
}

// Guards the load-bearing asymmetry: an empty query searches "." but the raw
// query still comes back. protojson omits both query:"" and score:0, so the
// fixture records their absence — a client reading query as absent and ""
// sees the same thing, which is the point.
func TestAssistSearchEmptyQueryFixture(t *testing.T) {
	checkAssistFixture(t, "search_empty_query.json", &consolev1.AssistFileSearchResponse{
		Root:  "/p",
		Query: "",
		Items: []*consolev1.FileSearchResult{
			{RelativePath: "AGENTS.md", AbsolutePath: "/p/AGENTS.md", Score: 0},
		},
	})
}

// The route itself emits the proto shape end to end.
func TestAssistSearchRouteEmitsProto(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	var sessions *services.SessionService
	routes.RegisterAssistRoutes(app, sessions, services.NewFsService(), services.NewSkillsService())

	resp, err := app.Test(httptest.NewRequest("GET", "/api/assist/search?q=&root="+dir, nil), 10000)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var envelope struct {
		Success bool                         `json:"success"`
		Data    *consolev1.AssistFileSearchResponse
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !envelope.Success || envelope.Data == nil {
		t.Fatal("search envelope missing data")
	}
	if envelope.Data.Root != dir {
		t.Errorf("root = %q, want %q", envelope.Data.Root, dir)
	}
	// The empty query must survive the round trip as "", not absent.
	if envelope.Data.Query != "" {
		t.Errorf("query = %q, want empty (raw query echoed verbatim)", envelope.Data.Query)
	}
	if len(envelope.Data.Items) == 0 {
		t.Error("expected AGENTS.md in a broad scan")
	}
}