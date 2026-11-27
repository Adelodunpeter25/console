package console

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
)

func newWebhookRuntime(t *testing.T) *Runtime {
	t.Helper()
	storage := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(storage, "global.db"), StorageDir: storage})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	runtime, err := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, QueueControl{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	runtime.webhooks = webhooks.New(database.Global())
	runtime.webhookBaseURL = func() string { return "https://example.test" }
	runtime.sessionID = "s1"
	return runtime
}

func TestConsoleWebhooksLifecycle(t *testing.T) {
	runtime := newWebhookRuntime(t)

	created := evalJSON(t, runtime,
		`console.webhooks.subscribe({topic:"ci", prompt:"fix build", filter:{status:"failed"}, once:true, expires_in_seconds:3600})`).(map[string]any)
	if created["topic"] != "ci" || created["status"] != "active" {
		t.Fatalf("created = %#v", created)
	}
	id := created["id"].(string)
	if created["webhook_url"] != "https://example.test/api/webhooks/ci/"+id {
		t.Fatalf("webhook_url = %v", created["webhook_url"])
	}

	list := evalJSON(t, runtime, `console.webhooks.list()`).([]any)
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	got := evalJSON(t, runtime, `console.webhooks.get({id:"`+id+`"})`).(map[string]any)
	if got["id"] != id {
		t.Fatalf("get = %#v", got)
	}

	unsub := evalJSON(t, runtime, `console.webhooks.unsubscribe({id:"`+id+`"})`).(map[string]any)
	if unsub["deleted"] != true {
		t.Fatalf("unsubscribe = %#v", unsub)
	}
	if _, err := runtime.Eval(t.Context(), `console.webhooks.get({id:"`+id+`"})`, time.Second); err == nil {
		t.Fatal("get after unsubscribe should fail")
	}
}

func TestConsoleWebhooksScopeRefusesOtherSession(t *testing.T) {
	runtime := newWebhookRuntime(t)
	created := evalJSON(t, runtime,
		`console.webhooks.subscribe({topic:"deploy", prompt:"check deploy"})`).(map[string]any)
	id := created["id"].(string)
	runtime.sessionID = "s2"
	for _, call := range []string{
		`console.webhooks.get({id:"` + id + `"})`,
		`console.webhooks.unsubscribe({id:"` + id + `"})`,
	} {
		if _, err := runtime.Eval(t.Context(), call, time.Second); err == nil {
			t.Fatalf("%s should be refused from another session", call)
		}
	}
}
