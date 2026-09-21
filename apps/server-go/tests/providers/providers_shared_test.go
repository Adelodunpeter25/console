// Shared provider helper coverage: SSE parsing, PKCE/JWT, credential
// files, JSON numbers, and function-call reassembly.
package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

func TestSharedParseSSE(t *testing.T) {
	body := "event: message\ndata: {\"type\":\"a\"}\n\ndata: [DONE]\n\ndata: not-json\n\ndata: {\"type\":\"b\"}\n\n"
	var got []string
	err := shared.ParseSSE(context.Background(), strings.NewReader(body), func(e map[string]any) error {
		got = append(got, e["type"].(string))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("events: %v", got)
	}
}

func TestSharedParseSSEContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := shared.ParseSSE(ctx, strings.NewReader("data: {\"type\":\"a\"}\n\n"), func(e map[string]any) error {
		return nil
	}); err == nil {
		t.Fatal("expected context error")
	}
}

func TestSharedPKCEAndJWT(t *testing.T) {
	verifier, challenge, err := shared.GeneratePKCE()
	if err != nil || verifier == "" || challenge == "" || verifier == challenge {
		t.Fatalf("pkce: %q %q %v", verifier, challenge, err)
	}
	payload, _ := json.Marshal(map[string]any{"sub": "123"})
	token := "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
	decoded := shared.DecodeJWTPayload(token)
	if decoded["sub"] != "123" {
		t.Fatalf("jwt: %v", decoded)
	}
	if shared.DecodeJWTPayload("bad") != nil {
		t.Fatal("malformed token must return nil")
	}
}

func TestSharedTokenError(t *testing.T) {
	err := shared.TokenError("Codex", 401, `{"error_description":"expired"}`)
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err: %v", err)
	}
	err = shared.TokenError("Codex", 500, "")
	if !strings.Contains(err.Error(), "unknown error") {
		t.Fatalf("err: %v", err)
	}
}

func TestSharedCredentialFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHARED_TEST_CREDS", dir+"/creds.json")
	path := shared.CredentialPath("SHARED_TEST_CREDS", "fallback.json")
	if path != dir+"/creds.json" {
		t.Fatalf("env override: %s", path)
	}
	if shared.CredentialFileExists(path) {
		t.Fatal("must not exist yet")
	}
	if err := shared.SaveCredentialFile(path, map[string]any{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if !shared.CredentialFileExists(path) {
		t.Fatal("must exist after save")
	}
}

func TestSharedNumbers(t *testing.T) {
	m := map[string]any{"f": 1.5, "i": 2, "n": json.Number("3.5"), "s": "x"}
	if v, ok := shared.NumberField(m, "f"); !ok || v != 1.5 {
		t.Fatalf("float: %v %v", v, ok)
	}
	if v, ok := shared.NumberField(m, "i"); !ok || v != 2 {
		t.Fatalf("int: %v %v", v, ok)
	}
	if v, ok := shared.NumberField(m, "n"); !ok || v != 3.5 {
		t.Fatalf("number: %v %v", v, ok)
	}
	if _, ok := shared.NumberField(m, "s"); ok {
		t.Fatal("string must not parse")
	}
	if shared.NumberFieldOr(m, "missing", 7) != 7 {
		t.Fatal("fallback failed")
	}
	if shared.ToolResultText("hi") != "hi" {
		t.Fatal("string passthrough failed")
	}
	if shared.ToolResultText(map[string]any{"a": 1}) != `{"a":1}` {
		t.Fatalf("json: %s", shared.ToolResultText(map[string]any{"a": 1}))
	}
}

func TestSharedAccumulator(t *testing.T) {
	acc := shared.NewAccumulator()
	// Out-of-order: delta + done before added.
	acc.Delta("item-1", `{"path":`)
	if fin := acc.Done("item-1", "listDir", `{"path":"."}`); fin != nil {
		t.Fatal("done before add must buffer")
	}
	fin := acc.Add("item-1", "call-1", "listDir", "")
	if fin == nil || fin.CallID != "call-1" || fin.Arguments != `{"path":"."}` {
		t.Fatalf("buffered finalize: %+v", fin)
	}
	// In-order: added, deltas, done.
	acc2 := shared.NewAccumulator()
	if fin := acc2.Add("item-2", "call-2", "t", ""); fin != nil {
		t.Fatal("fresh add must not finalize")
	}
	acc2.Delta("item-2", `{"a":`)
	acc2.Delta("item-2", `1}`)
	fin = acc2.Done("item-2", "", `{"a":1}`)
	if fin == nil || fin.Name != "t" || fin.Arguments != `{"a":1}` {
		t.Fatalf("in-order: %+v", fin)
	}
	if len(acc2.Unfinalized()) != 0 {
		t.Fatal("finalized call must not flush")
	}
	// Incomplete: deltas only, flushed at end.
	acc3 := shared.NewAccumulator()
	acc3.Add("item-3", "call-3", "t", "")
	acc3.Delta("item-3", `{"x":1}`)
	unfin := acc3.Unfinalized()
	if len(unfin) != 1 || unfin[0].Arguments != `{"x":1}` {
		t.Fatalf("unfinalized: %+v", unfin)
	}
}
