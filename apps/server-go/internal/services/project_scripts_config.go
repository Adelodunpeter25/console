// console.toml script parsing. Port of
// apps/server/api/src/services/project-scripts/config.ts.
package services

import (
	"fmt"
	"regexp"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/BurntSushi/toml"
)

const (
	scriptMaxLabel   = 200
	scriptMaxCommand = 4096
	scriptCacheTTL   = 2 * time.Second
)

var (
	scriptIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	scriptShortcutPat = regexp.MustCompile(`(?i)^(cmd|ctrl|alt|shift)(-(cmd|ctrl|alt|shift))*-[a-z0-9]+$`)
)

type scriptEntry struct {
	Label      *string `toml:"label"`
	Command    *string `toml:"command"`
	Shortcut   *string `toml:"shortcut"`
	Persistent *bool   `toml:"persistent"`
}

type scriptDoc struct {
	Scripts map[string]scriptEntry `toml:"scripts"`
}

func parseProjectScripts(text string) ([]types.ProjectScript, error) {
	fail := func(msg string) error {
		return fmt.Errorf("Invalid console.toml: %s", msg)
	}
	var doc scriptDoc
	if _, err := toml.Decode(text, &doc); err != nil {
		return nil, fail(err.Error())
	}
	scripts := make([]types.ProjectScript, 0)
	for id, entry := range doc.Scripts {
		if !scriptIDPattern.MatchString(id) {
			return nil, fail(fmt.Sprintf("script identifier '%s' contains unsupported characters.", id))
		}
		if entry.Label == nil || *entry.Label == "" || len(*entry.Label) > scriptMaxLabel {
			return nil, fail(fmt.Sprintf("scripts.%s.label must be a non-empty string.", id))
		}
		if entry.Command == nil || *entry.Command == "" || len(*entry.Command) > scriptMaxCommand {
			return nil, fail(fmt.Sprintf("scripts.%s.command must be a non-empty string.", id))
		}
		var shortcut *string
		if entry.Shortcut != nil {
			if *entry.Shortcut == "" || len(*entry.Shortcut) > 100 || !scriptShortcutPat.MatchString(*entry.Shortcut) {
				return nil, fail(fmt.Sprintf("scripts.%s.shortcut has unsupported syntax.", id))
			}
			shortcut = entry.Shortcut
		}
		persistent := entry.Persistent != nil && *entry.Persistent
		scripts = append(scripts, types.ProjectScript{
			ID: id, Label: *entry.Label, Command: *entry.Command,
			Shortcut: shortcut, Persistent: persistent,
		})
	}
	return scripts, nil
}

// ParseProjectScriptsForTest exposes the parser to the external tests package.
func ParseProjectScriptsForTest(text string) ([]types.ProjectScript, error) {
	return parseProjectScripts(text)
}
