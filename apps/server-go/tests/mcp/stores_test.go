// MCP server config + credential store coverage.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func httpServer(id string) mcp.ServerConfig {
	return mcp.ServerConfig{
		ID: id, Label: "Atlassian", Transport: mcp.TransportHTTP,
		URL:     "https://mcp.atlassian.com/v2/mcp",
		Auth:    &mcp.AuthConfig{Type: mcp.AuthOAuth2, TokenRef: id},
		Enabled: true,
	}
}

func TestConfigCRUDPersists(t *testing.T) {
	dir := t.TempDir()
	store := mcp.NewConfigStore(dir)

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
	if _, err := store.Save(mcp.ServerConfig{
		ID: "local-fs", Label: "FS", Transport: mcp.TransportStdio, Command: "npx",
	}); err != nil {
		t.Fatalf("save stdio: %v", err)
	}

	// A fresh store instance reads the same file back.
	again := mcp.NewConfigStore(dir)
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

func TestConfigValidation(t *testing.T) {
	store := mcp.NewConfigStore(t.TempDir())
	bad := map[string]mcp.ServerConfig{
		"bad id":        {ID: "a b", Label: "x", Transport: "http", URL: "u"},
		"no label":      {ID: "a", Transport: "http", URL: "u"},
		"http no url":   {ID: "a", Label: "x", Transport: "http"},
		"stdio no cmd":  {ID: "a", Label: "x", Transport: "stdio"},
		"bad transport": {ID: "a", Label: "x", Transport: "sse", URL: "u"},
		"bad auth type": {ID: "a", Label: "x", Transport: "http", URL: "u", Auth: &mcp.AuthConfig{Type: "magic"}},
		"auth no ref":   {ID: "a", Label: "x", Transport: "http", URL: "u", Auth: &mcp.AuthConfig{Type: "oauth2"}},
		"bad tier":      {ID: "a", Label: "x", Transport: "http", URL: "u", TierOverrides: map[string]string{"t": "root"}},
	}
	for name, cfg := range bad {
		if _, err := store.Save(cfg); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestCredentialStore(t *testing.T) {
	dir := t.TempDir()
	store := mcp.NewCredentialStore(dir)

	if _, ok, err := store.Get("atlassian"); ok || err != nil {
		t.Fatalf("empty get: ok=%v err=%v", ok, err)
	}
	if err := store.Set("atlassian", mcp.Credential{
		Kind: mcp.CredentialOAuth, ClientID: "cid", AccessToken: "at", RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("set oauth: %v", err)
	}
	if err := store.Set("token", mcp.Credential{Kind: mcp.CredentialStatic, Header: "Bearer x"}); err != nil {
		t.Fatalf("set static: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "mcp-credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credential file perm = %o, want 600", perm)
	}

	again := mcp.NewCredentialStore(dir)
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

func TestCredentialValidation(t *testing.T) {
	store := mcp.NewCredentialStore(t.TempDir())
	if err := store.Set("bad ref", mcp.Credential{Kind: "oauth2"}); err == nil {
		t.Error("bad ref accepted")
	}
	if err := store.Set("a", mcp.Credential{Kind: "static"}); err == nil {
		t.Error("static without header accepted")
	}
	if err := store.Set("a", mcp.Credential{Kind: "nope"}); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestConfigFileHoldsNoSecrets(t *testing.T) {
	dir := t.TempDir()
	mcp.NewConfigStore(dir).Save(httpServer("atlassian"))
	mcp.NewCredentialStore(dir).Set("atlassian", mcp.Credential{
		Kind: mcp.CredentialOAuth, AccessToken: "super-secret-token",
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
