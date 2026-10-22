package visualize

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fixtureRoot is the repository root, reached from this package directory.
const fixtureRoot = "../../../../.."

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{fixtureRoot}, parts...)...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// The shared golden specs are the cross-client contract: Go compiles them,
// Kotlin and Rust decode them. They must compile cleanly, keep their meaning
// through the canonical form, and compile to the same bytes every time.
func TestGoldenSpecFixtures(t *testing.T) {
	for _, name := range []string{"line", "dashboard"} {
		t.Run(name, func(t *testing.T) {
			raw := readFixture(t, "proto", "testdata", "visual", name+".json")
			var input map[string]any
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			canonical, spec, err := Compile(input)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			var want, got any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(canonical, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("canonical spec changed the fixture:\n got %s\nwant %s", canonical, raw)
			}
			again, _, err := Compile(input)
			if err != nil {
				t.Fatalf("Compile again: %v", err)
			}
			if string(again) != string(canonical) {
				t.Fatalf("canonical bytes are not stable:\n%s\n%s", again, canonical)
			}
			if Summary(spec) == "" {
				t.Fatal("summary is empty")
			}
		})
	}
}

// The tool-result fixture carries the canonical line spec as base64 bytes, so
// the wire fixture and the spec fixture can never drift apart.
func TestGoldenToolResultCarriesGoldenSpec(t *testing.T) {
	raw := readFixture(t, "proto", "testdata", "visual", "line.json")
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	canonical, _, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	fixture := readFixture(t, "proto", "testdata", "message", "toolresult_visual.json")
	var message struct {
		ToolResult struct {
			Results []struct {
				Visual struct {
					Version uint32 `json:"version"`
					Spec    string `json:"spec"`
				} `json:"visual"`
			} `json:"results"`
		} `json:"toolResult"`
	}
	if err := json.Unmarshal(fixture, &message); err != nil {
		t.Fatalf("decode tool result fixture: %v", err)
	}
	results := message.ToolResult.Results
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Visual.Version != SpecVersion {
		t.Errorf("visual version = %d, want %d", results[0].Visual.Version, SpecVersion)
	}
	decoded, err := base64.StdEncoding.DecodeString(results[0].Visual.Spec)
	if err != nil {
		t.Fatalf("decode spec bytes: %v", err)
	}
	if string(decoded) != string(canonical) {
		t.Fatalf("fixture spec drifted:\n got %s\nwant %s", decoded, canonical)
	}
}
