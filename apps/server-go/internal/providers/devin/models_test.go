package devin

import (
	"testing"

	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
)

func effortEntry(key, name string) *exav1.ModelFamilyMetadataEntry {
	return &exav1.ModelFamilyMetadataEntry{
		Key:   key,
		Value: &exav1.ModelFamilyMetadataValue{Name: name},
	}
}

func familyConfig(uid, label string, defaultInFamily bool, entries ...*exav1.ModelFamilyMetadataEntry) *exav1.ClientModelConfig {
	return &exav1.ClientModelConfig{
		ModelUid:               uid,
		MaxTokens:              200000,
		IsDefaultModelInFamily: defaultInFamily,
		ModelFamilyMetadata: &exav1.ModelFamilyMetadata{
			ModelFamilyLabel: label,
			Entries:          entries,
		},
	}
}

func TestNormalizeDevinModelsCollapsesEffortFamily(t *testing.T) {
	configs := []*exav1.ClientModelConfig{
		familyConfig("claude-a-low", "Claude A", true, effortEntry("Effort", "Low")),
		familyConfig("claude-a-high", "Claude A", false, effortEntry("Reasoning Effort", "XHigh")),
		familyConfig("claude-a-off", "Claude A", false, effortEntry("Effort", "None")),
		{ModelUid: "swe-1-6", MaxTokens: 1000},
	}
	models, meta, err := normalizeDevinModels(configs)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]bool{}
	for _, m := range models {
		byID[m.ID] = true
	}
	if !byID["claude-a"] || !byID["swe-1-6"] {
		t.Fatalf("models: %+v", models)
	}
	for _, id := range []string{"claude-a-low", "claude-a-high", "claude-a-off"} {
		if byID[id] {
			t.Fatalf("family member %q leaked", id)
		}
	}

	// Seed the discovery cache so chatModelUID resolves through modelMetaFor.
	liveMeta = map[string]modelMeta{}
	for id, m := range meta {
		liveMeta[id] = m
	}
	defer func() { liveMeta = map[string]modelMeta{} }()

	if uid, m := chatModelUID("claude-a", "xhigh"); uid != "claude-a-high" || m.isAssignRouter {
		t.Fatalf("xhigh effort uid: %q", uid)
	}
	// "high" has no route in this family: falls back to the default member.
	if uid, _ := chatModelUID("claude-a", "high"); uid != "claude-a-low" {
		t.Fatalf("unrouted level uid: %q", uid)
	}
	if uid, _ := chatModelUID("claude-a", "none"); uid != "claude-a-off" {
		t.Fatalf("none effort uid: %q", uid)
	}
	// Empty level falls back to the family's default member.
	if uid, _ := chatModelUID("claude-a", ""); uid != "claude-a-low" {
		t.Fatalf("default uid: %q", uid)
	}
	// Unknown level also falls back to the default member.
	if uid, _ := chatModelUID("claude-a", "bogus"); uid != "claude-a-low" {
		t.Fatalf("unknown-level uid: %q", uid)
	}
	// Standalone models ignore the level entirely.
	if uid, _ := chatModelUID("swe-1-6", "high"); uid != "swe-1-6" {
		t.Fatalf("standalone uid: %q", uid)
	}
}
