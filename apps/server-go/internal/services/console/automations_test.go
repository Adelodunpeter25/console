// The console.automation.* methods: create/get/update/delete against the
// schedule store, scoped to the calling session.
package console

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

func newAutomationRuntime(t *testing.T) *Runtime {
	t.Helper()
	storage := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(storage, "global.db"), StorageDir: storage})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	runtime, err := NewRuntime(nil, nil, nil, nil, nil, automations.New(database.Global()), nil, nil, nil, nil, QueueControl{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	runtime.sessionID = "s1"
	return runtime
}

func TestConsoleAutomationLifecycle(t *testing.T) {
	runtime := newAutomationRuntime(t)
	runAt := fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix())
	created := evalJSON(t, runtime,
		`console.automation.create({prompt:"remind me", kind:"once", run_at:`+runAt+`})`).(map[string]any)
	if created["prompt"] != "remind me" || created["status"] != "active" {
		t.Fatalf("created = %#v", created)
	}
	id := created["id"].(string)

	got := evalJSON(t, runtime, `console.automation.get({id:"`+id+`"})`).(map[string]any)
	if got["id"] != id {
		t.Fatalf("get = %#v", got)
	}

	// One active automation per session.
	if _, err := runtime.Eval(t.Context(),
		`console.automation.create({prompt:"another", kind:"interval", interval_seconds:60})`, time.Second); err == nil {
		t.Fatal("second create should be refused")
	}

	updated := evalJSON(t, runtime,
		`console.automation.update({id:"`+id+`", prompt:"remind me again"})`).(map[string]any)
	if updated["prompt"] != "remind me again" {
		t.Fatalf("update = %#v", updated)
	}

	deleted := evalJSON(t, runtime, `console.automation.delete({id:"`+id+`"})`).(map[string]any)
	if deleted["deleted"] != true {
		t.Fatalf("delete = %#v", deleted)
	}
	if _, err := runtime.Eval(t.Context(), `console.automation.get({id:"`+id+`"})`, time.Second); err == nil {
		t.Fatal("get after delete should fail")
	}
}

func TestConsoleAutomationScopeRefusesOtherSession(t *testing.T) {
	runtime := newAutomationRuntime(t)
	// Create on s1, then pretend to be s2.
	created := evalJSON(t, runtime,
		`console.automation.create({prompt:"mine", kind:"interval", interval_seconds:300})`).(map[string]any)
	id := created["id"].(string)
	runtime.sessionID = "s2"
	for _, call := range []string{
		`console.automation.get({id:"` + id + `"})`,
		`console.automation.update({id:"` + id + `", prompt:"x"})`,
		`console.automation.delete({id:"` + id + `"})`,
	} {
		if _, err := runtime.Eval(t.Context(), call, time.Second); err == nil {
			t.Fatalf("%s should be refused from another session", call)
		}
	}
}
