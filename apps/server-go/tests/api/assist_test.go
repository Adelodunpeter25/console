// Assist commands coverage: the composer autocomplete must suggest
// /computer-use, or the user can never discover the command. Skills come from
// the temp dir (none), so the only commands are the builtins.
package tests

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func assistCommands(t *testing.T) []map[string]any {
	t.Helper()
	app := fiber.New()
	// Sessions and fs are untouched on the param-less route: no session id
	// means no session lookup, and file search is a different endpoint.
	var sessions *services.SessionService
	routes.RegisterAssistRoutes(app, sessions, services.NewFsService(), services.NewSkillsService())
	req := httptest.NewRequest("GET", "/api/assist/commands", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("GET commands: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var envelope struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !envelope.Success {
		t.Fatal("success = false")
	}
	return envelope.Data
}

// TestAssistListsComputerUse is the discoverability half of /computer-use:
// the command must appear in autocomplete. (The other half — the model
// learning nothing until invocation — is covered by the interception tests,
// since no skill or tool mentions computer use up front.)
func TestAssistListsComputerUse(t *testing.T) {
	var found map[string]any
	for _, cmd := range assistCommands(t) {
		if cmd["name"] == "computer-use" {
			found = cmd
		}
	}
	if found == nil {
		t.Fatal("/computer-use not listed in assist commands")
	}
	if found["builtin"] != true {
		t.Errorf("computer-use builtin = %v, want true", found["builtin"])
	}
	if desc, _ := found["description"].(string); desc == "" {
		t.Error("computer-use needs a description for the autocomplete card")
	}
}

// TestAssistKeepsInitCommand guards the existing builtin while adding one.
func TestAssistKeepsInitCommand(t *testing.T) {
	for _, cmd := range assistCommands(t) {
		if cmd["name"] == "init" {
			return
		}
	}
	t.Fatal("/init went missing from assist commands")
}
