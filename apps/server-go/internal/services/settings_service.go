// Model-role settings persisted to settings.json. Port of
// apps/server/agent/src/service/model-roles.ts (load/save/patch semantics).
package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

var settingsRoles = []string{"vision", "smol"}

type Settings struct {
	ModelRoles map[string]string `json:"modelRoles"`
}

type SettingsService struct{}

func NewSettingsService() *SettingsService { return &SettingsService{} }

func settingsPath() string {
	if p := osGetenv("CONSOLE_SETTINGS_PATH"); p != "" {
		return p
	}
	return filepath.Join(utils.ConsoleStorageDir(), "settings.json")
}

func (s *SettingsService) Load() Settings {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{ModelRoles: map[string]string{}}
	}
	var parsed Settings
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Settings{ModelRoles: map[string]string{}}
	}
	if parsed.ModelRoles == nil {
		parsed.ModelRoles = map[string]string{}
	}
	// Keep only known roles with non-empty values.
	clean := map[string]string{}
	for _, role := range settingsRoles {
		if v := strings.TrimSpace(parsed.ModelRoles[role]); v != "" {
			clean[role] = v
		}
	}
	return Settings{ModelRoles: clean}
}

// IsModelRole reports whether the role is a known console model role.
func IsModelRole(role string) bool {
	for _, r := range settingsRoles {
		if r == role {
			return true
		}
	}
	return false
}

// Patch merges the patch onto current roles: mentioned roles are set
// (empty/null clears), unmentioned roles are left untouched.
func (s *SettingsService) Patch(patch map[string]*string) (Settings, error) {
	current := s.Load()
	merged := map[string]string{}
	for k, v := range current.ModelRoles {
		merged[k] = v
	}
	for _, role := range settingsRoles {
		value, ok := patch[role]
		if !ok {
			continue
		}
		if value == nil || strings.TrimSpace(*value) == "" {
			delete(merged, role)
			continue
		}
		merged[role] = strings.TrimSpace(*value)
	}
	result := Settings{ModelRoles: merged}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return current, err
	}
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return current, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return current, err
	}
	return result, nil
}
