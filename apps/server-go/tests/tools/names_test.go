package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestRegistryToleratesAliases(t *testing.T) {
	reg := tools.NewRegistry(tools.DefaultTools()...)

	// Both canonical camelCase and legacy snake_case resolve to the same tool
	for _, pair := range [][2]string{
		{"readFile", "read_file"},
		{"writeFile", "write_file"},
		{"listDir", "list_dir"},
		{"editFile", "edit_file"},
		{"batchWrite", "batch_write"},
		{"webSearch", "web_search"},
		{"webFetch", "web_fetch"},
		{"bashJob", "bash_job"},
		{"askMany", "ask_many"},
	} {
		canon, legacy := pair[0], pair[1]
		tCanon, err1 := reg.Get(canon)
		tLegacy, err2 := reg.Get(legacy)
		if err1 != nil {
			t.Errorf("Get(%q) failed: %v", canon, err1)
		}
		if err2 != nil {
			t.Errorf("Get(%q) failed: %v", legacy, err2)
		}
		if tCanon != tLegacy {
			t.Errorf("Get(%q) != Get(%q)", canon, legacy)
		}
	}
}

func TestCanonicalNameHelper(t *testing.T) {
	if got := tools.CanonicalName("read_file"); got != "readFile" {
		t.Fatalf("CanonicalName(read_file) = %q, want readFile", got)
	}
	if got := tools.CanonicalName("readFile"); got != "readFile" {
		t.Fatalf("CanonicalName(readFile) = %q, want readFile", got)
	}
	if got := tools.CanonicalName("write_plan"); got != "writePlan" {
		t.Fatalf("CanonicalName(write_plan) = %q, want writePlan", got)
	}
	if got := tools.CanonicalName("request_full_access"); got != "requestFullAccess" {
		t.Fatalf("CanonicalName(request_full_access) = %q, want requestFullAccess", got)
	}
}
