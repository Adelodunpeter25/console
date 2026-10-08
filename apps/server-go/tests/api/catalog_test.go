// Provider catalog migration coverage: proto-built catalog entries and the
// per-provider model list must match the shared golden fixtures byte for
// byte (envelope keys alphabetical, payload keys in proto field order).
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func catalogFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "catalog", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func catalogEntriesToProto(entries []types.ProviderEntry) []*consolev1.ProviderCatalogEntry {
	out := make([]*consolev1.ProviderCatalogEntry, 0, len(entries))
	for _, e := range entries {
		models := make([]*consolev1.Model, 0, len(e.Models))
		for _, m := range e.Models {
			levels := append([]string(nil), m.ThinkingLevels...)
			entry := &consolev1.Model{
				Id: m.ID, Provider: m.Provider,
				ContextWindow: uint32(m.ContextWindow), SupportsImages: m.SupportsImages,
				SupportedThinkingLevels: levels,
			}
			if m.DefaultThinking != "" {
				entry.DefaultThinkingLevel = &m.DefaultThinking
			}
			models = append(models, entry)
		}
		out = append(out, &consolev1.ProviderCatalogEntry{
			Name: e.Name, DisplayName: e.DisplayName, Description: e.Description,
			Models: models, AuthMethod: e.AuthMethod,
		})
	}
	return out
}

func TestProviderCatalogProtoMatchesFixture(t *testing.T) {
	entries := catalogEntriesToProto([]types.ProviderEntry{
		{
			Name: "codex", DisplayName: "Codex", Description: "OpenAI", AuthMethod: "oauth",
			Models: []types.Model{
				{ID: "gpt-5", Provider: "codex", ContextWindow: 272000, SupportsImages: true,
					ThinkingLevels: []string{"low", "high"}, DefaultThinking: "medium"},
				{ID: "gpt-5-mini", Provider: "codex", ContextWindow: 128000},
			},
		},
		{Name: "empty", DisplayName: "Empty", Description: "no models", AuthMethod: "none",
			Models: []types.Model{}},
	})

	elements := make([]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		raw, err := protojson.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, raw)
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != catalogFixture(t, "providers.json") {
		t.Fatalf("catalog drifted:\n got %s\nwant %s", encoded, catalogFixture(t, "providers.json"))
	}

	// A provider with no models omits the key: never null (which used to
	// force a null-tolerant deserializer on the desktop) and never [].
	var decoded []consolev1.ProviderCatalogEntry
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded[1].Models) != 0 {
		t.Fatalf("empty provider must decode to no models: %+v", decoded[1])
	}
	if strings.Contains(string(encoded), `"name":"empty","displayName":"Empty","description":"no models","models"`) {
		t.Fatalf("empty model list must omit the key: %s", encoded)
	}
}

func TestProviderModelsResponseMatchesFixture(t *testing.T) {
	msg := &consolev1.ProviderModelsResponse{
		Provider: "codex",
		Models: []*consolev1.Model{
			{Id: "gpt-5", Provider: "codex", ContextWindow: 272000},
		},
	}
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != catalogFixture(t, "provider_models.json") {
		t.Fatalf("models response drifted:\n got %s\nwant %s", compactJSON(t, raw), catalogFixture(t, "provider_models.json"))
	}

	// Counts stay JSON numbers (unlike int64 timestamps elsewhere).
	if !strings.Contains(string(raw), `"contextWindow":272000`) {
		t.Fatalf("contextWindow must be a number: %s", raw)
	}
	var _ proto.Message = msg
}
