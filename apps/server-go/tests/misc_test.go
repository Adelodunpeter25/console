// Settings + skills discovery coverage.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestSettingsPatchSemantics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_SETTINGS_PATH", filepath.Join(dir, "settings.json"))
	settings := services.NewSettingsService()

	// Defaults on missing file.
	if roles := settings.Load().ModelRoles; len(roles) != 0 {
		t.Fatalf("default roles: %v", roles)
	}

	// Set vision.
	vision := "claude-opus-4"
	result, err := settings.Patch(map[string]*string{"vision": &vision})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if result.ModelRoles["vision"] != "claude-opus-4" {
		t.Fatalf("vision: %v", result.ModelRoles)
	}

	// Partial patch keeps unmentioned roles (merged, not replaced).
	smol := "gpt-5-mini"
	result, err = settings.Patch(map[string]*string{"smol": &smol})
	if err != nil {
		t.Fatalf("patch 2: %v", err)
	}
	if result.ModelRoles["vision"] != "claude-opus-4" || result.ModelRoles["smol"] != "gpt-5-mini" {
		t.Fatalf("merged: %v", result.ModelRoles)
	}

	// Empty value clears the role.
	empty := ""
	result, _ = settings.Patch(map[string]*string{"vision": &empty})
	if _, present := result.ModelRoles["vision"]; present {
		t.Fatalf("vision should be cleared: %v", result.ModelRoles)
	}
	if result.ModelRoles["smol"] != "gpt-5-mini" {
		t.Fatalf("smol must survive: %v", result.ModelRoles)
	}

	// Persists across reload.
	if reloaded := settings.Load().ModelRoles; reloaded["smol"] != "gpt-5-mini" {
		t.Fatalf("reload: %v", reloaded)
	}
}

func TestSkillDiscovery(t *testing.T) {
	skills := services.NewSkillsService()
	root := t.TempDir()
	projectSkills := filepath.Join(root, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(projectSkills, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(projectSkills, "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy the app\n---\nBody")
	write(filepath.Join(projectSkills, "quick.md"),
		"---\ndescription: Quick fix\n---\nBody")
	// Hidden skill excluded.
	write(filepath.Join(projectSkills, "secret.md"),
		"---\ndescription: Hidden\nhide: true\n---\nBody")

	found := skills.Discover(root)
	byName := map[string]string{}
	for _, s := range found {
		byName[s.Name] = s.Description
	}
	if byName["deploy"] != "Deploy the app" {
		t.Fatalf("deploy skill: %+v", byName)
	}
	if byName["quick"] != "Quick fix" {
		t.Fatalf("quick skill: %+v", byName)
	}
	if _, ok := byName["secret"]; ok {
		t.Fatal("hidden skill must not be discovered")
	}
}
