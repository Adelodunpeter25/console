// MCP server config + credential store coverage.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func httpServer(id string) services.MCPServerConfig {
	return services.MCPServerConfig{
		ID: id, Label: "Atlassian", Transport: services.MCPTransportHTTP,
		URL:     "https://mcp.atlassian.com/v2/mcp",
		Auth:    &services.MCPAuthConfig{Type: services.MCPAuthOAuth2, TokenRef: id},
		Enabled: true,
	}
}

func TestMCPConfigCRUDPersists(t *testing.T) {
	dir := t.TempDir()
	store := services.NewMCPConfigStore(dir)

	if list, err := store.List(); err != nil || len(list) != 0 {
		t.Fatalf("empty store: list=%v err=%v", list, err)
	}
	saved, err := store.Save(httpServer("atlassian"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.CreatedAt == 0 || saved.UpdatedAt == 0 {
		t.Fatalf("timestamps not set: %+v", saved)
	}
	if _, err := store.Save(services.MCPServerConfig{
		ID: "local-fs", Label: "FS", Transport: services.MCPTransportStdio, Command: "npx",
	}); err != nil {
		t.Fatalf("save stdio: %v", err)
	}

	// A fresh store instance reads the same file back.
	again := services.NewMCPConfigStore(dir)
	list, err := again.List()
	if err != nil || len(list) != 2 || list[0].ID != "atlassian" || list[1].ID != "local-fs" {
		t.Fatalf("reload: list=%+v err=%v", list, err)
	}

	// Update keeps CreatedAt.
	upd := httpServer("atlassian")
	upd.Label = "Atlassian Cloud"
	updated, _ := again.Save(upd)
	if updated.CreatedAt != saved.CreatedAt || updated.Label != "Atlassian Cloud" {
		t.Fatalf("update: %+v vs %+v", updated, saved)
	}

	if ok, err := again.Delete("atlassian"); !ok || err != nil {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	if ok, _ := again.Delete("atlassian"); ok {
		t.Fatal("second delete should report not found")
	}
	if _, ok, _ := again.Get("atlassian"); ok {
		t.Fatal("deleted server still present")
	}
}

func TestMCPConfigValidation(t *testing.T) {
	store := services.NewMCPConfigStore(t.TempDir())
	bad := map[string]services.MCPServerConfig{
		"bad id":        {ID: "a b", Label: "x", Transport: "http", URL: "u"},
		"no label":      {ID: "a", Transport: "http", URL: "u"},
		"http no url":   {ID: "a", Label: "x", Transport: "http"},
		"stdio no cmd":  {ID: "a", Label: "x", Transport: "stdio"},
		"bad transport": {ID: "a", Label: "x", Transport: "sse", URL: "u"},
		"bad auth type": {ID: "a", Label: "x", Transport: "http", URL: "u", Auth: &services.MCPAuthConfig{Type: "magic"}},
		"auth no ref":   {ID: "a", Label: "x", Transport: "http", URL: "u", Auth: &services.MCPAuthConfig{Type: "oauth2"}},
		"bad tier":      {ID: "a", Label: "x", Transport: "http", URL: "u", TierOverrides: map[string]string{"t": "root"}},
	}
	for name, cfg := range bad {
		if _, err := store.Save(cfg); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestMCPCredentialStore(t *testing.T) {
	dir := t.TempDir()
	store := services.NewMCPCredentialStore(dir)

	if _, ok, err := store.Get("atlassian"); ok || err != nil {
		t.Fatalf("empty get: ok=%v err=%v", ok, err)
	}
	if err := store.Set("atlassian", services.MCPCredential{
		Kind: services.MCPCredentialOAuth, ClientID: "cid", AccessToken: "at", RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("set oauth: %v", err)
	}
	if err := store.Set("token", services.MCPCredential{Kind: services.MCPCredentialStatic, Header: "Bearer x"}); err != nil {
		t.Fatalf("set static: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "mcp-credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credential file perm = %o, want 600", perm)
	}

	again := services.NewMCPCredentialStore(dir)
	got, ok, _ := again.Get("atlassian")
	if !ok || got.ClientID != "cid" || got.RefreshToken != "rt" {
		t.Fatalf("reload: %+v ok=%v", got, ok)
	}

	if ok, _ := again.Delete("atlassian"); !ok {
		t.Fatal("delete should find the credential")
	}
	if _, ok, _ := again.Get("atlassian"); ok {
		t.Fatal("credential survived delete")
	}
	if _, ok, _ := again.Get("token"); !ok {
		t.Fatal("delete removed the wrong credential")
	}
}

func TestMCPCredentialValidation(t *testing.T) {
	store := services.NewMCPCredentialStore(t.TempDir())
	if err := store.Set("bad ref", services.MCPCredential{Kind: "oauth2"}); err == nil {
		t.Error("bad ref accepted")
	}
	if err := store.Set("a", services.MCPCredential{Kind: "static"}); err == nil {
		t.Error("static without header accepted")
	}
	if err := store.Set("a", services.MCPCredential{Kind: "nope"}); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestMCPConfigFileHoldsNoSecrets(t *testing.T) {
	dir := t.TempDir()
	services.NewMCPConfigStore(dir).Save(httpServer("atlassian"))
	services.NewMCPCredentialStore(dir).Set("atlassian", services.MCPCredential{
		Kind: services.MCPCredentialOAuth, AccessToken: "super-secret-token",
	})
	raw, _ := os.ReadFile(filepath.Join(dir, "mcp-servers.json"))
	if string(raw) == "" || contains(string(raw), "super-secret-token") {
		t.Fatal("mcp-servers.json must not contain credentials")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
