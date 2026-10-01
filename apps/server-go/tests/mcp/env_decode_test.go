// ServerConfig.Env decoding: the object form the store writes and the
// pair-array form the desktop client posts must both decode, because a shape
// difference is not a reason to reject a save.
package tests

import (
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func decodeServerConfig(t *testing.T, body string) mcp.ServerConfig {
	t.Helper()
	var cfg mcp.ServerConfig
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return cfg
}

func TestServerConfigEnvAcceptsObjectForm(t *testing.T) {
	cfg := decodeServerConfig(t, `{"id":"a","env":{"TOKEN":"secret","REGION":"eu"}}`)
	if cfg.Env["TOKEN"] != "secret" || cfg.Env["REGION"] != "eu" {
		t.Fatalf("object form not decoded: %v", cfg.Env)
	}
}

func TestServerConfigEnvAcceptsPairArrayForm(t *testing.T) {
	// What the desktop client posts: Vec<(String, String)> serialized as
	// [["TOKEN","secret"],["REGION","eu"]]. This shape used to 400.
	cfg := decodeServerConfig(t, `{"id":"a","env":[["TOKEN","secret"],["REGION","eu"]]}`)
	if cfg.Env["TOKEN"] != "secret" || cfg.Env["REGION"] != "eu" {
		t.Fatalf("pair-array form not decoded: %v", cfg.Env)
	}
}

func TestServerConfigEnvAcceptsEmptyPairArray(t *testing.T) {
	// The reported failure: an env-less server still posts "env": [].
	cfg := decodeServerConfig(t, `{"id":"a","env":[]}`)
	if len(cfg.Env) != 0 {
		t.Fatalf("empty array should decode to no vars: %v", cfg.Env)
	}
}

func TestServerConfigEnvAcceptsNullAndOmitted(t *testing.T) {
	if env := decodeServerConfig(t, `{"id":"a","env":null}`).Env; len(env) != 0 {
		t.Fatalf("null should decode to no vars: %v", env)
	}
	if env := decodeServerConfig(t, `{"id":"a"}`).Env; len(env) != 0 {
		t.Fatalf("omitted should decode to no vars: %v", env)
	}
}

func TestServerConfigEnvPairArrayPreservesLastDuplicate(t *testing.T) {
	// Later wins, matching how an object with repeated work would resolve.
	cfg := decodeServerConfig(t, `{"id":"a","env":[["K","first"],["K","last"]]}`)
	if cfg.Env["K"] != "last" {
		t.Fatalf("expected last duplicate to win, got %q", cfg.Env["K"])
	}
}

func TestServerConfigEnvRejectsUnusableShape(t *testing.T) {
	var cfg mcp.ServerConfig
	err := json.Unmarshal([]byte(`{"id":"a","env":"TOKEN=secret"}`), &cfg)
	if err == nil {
		t.Fatal("expected an error for a bare string env")
	}
}

func TestServerConfigEnvAlwaysEncodesAsObject(t *testing.T) {
	// Both accepted shapes must round-trip to the canonical object form, so
	// stored configs and API responses stay one shape.
	for _, body := range []string{
		`{"id":"a","env":{"K":"V"}}`,
		`{"id":"a","env":[["K","V"]]}`,
	} {
		cfg := decodeServerConfig(t, body)
		out, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var round mcp.ServerConfig
		if err := json.Unmarshal(out, &round); err != nil {
			t.Fatalf("re-decode %s: %v", out, err)
		}
		if round.Env["K"] != "V" {
			t.Fatalf("env did not round-trip from %s: %s", body, out)
		}
		var obj map[string]any
		if err := json.Unmarshal(out, &obj); err != nil {
			t.Fatalf("not an object: %v", err)
		}
		if _, isArray := obj["env"].([]any); isArray {
			t.Fatalf("env encoded as an array, want object: %s", out)
		}
	}
}
