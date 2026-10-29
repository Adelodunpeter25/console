package console

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func newConsoleSessionRuntime(t *testing.T, active func(string) bool) (*Runtime, *services.SessionService) {
	t.Helper()
	storage := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(storage, "global.db"), StorageDir: storage})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	sessions := services.NewSessionService(database)
	projects := services.NewProjectService(database)
	runtime, err := NewRuntime(nil, nil, sessions, projects, active, nil, nil, nil, QueueControl{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return runtime, sessions
}

func evalJSON(t *testing.T, runtime *Runtime, code string) any {
	t.Helper()
	value, err := runtime.Eval(context.Background(), code, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parts, ok := value.([]map[string]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("evaluation result = %#v", value)
	}
	text, ok := parts[len(parts)-1]["text"].(string)
	if !ok {
		t.Fatalf("evaluation text = %#v", parts[len(parts)-1])
	}
	// Methods return their data directly, so the final text is the data itself.
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("evaluation text is not JSON: %q: %v", text, err)
	}
	return decoded
}

func TestConsoleSessionLifecycle(t *testing.T) {
	runtime, sessions := newConsoleSessionRuntime(t, nil)
	created := evalJSON(t, runtime, `console.sessions.create({prompt:"inspect", cwd:"/tmp"})`).(map[string]any)
	session, ok := created["session"].(map[string]any)
	if !ok {
		t.Fatalf("created session = %#v", created["session"])
	}
	id := session["id"].(string)

	listed := evalJSON(t, runtime, `console.sessions.list({})`).([]any)
	if len(listed) != 1 {
		t.Fatalf("session list length = %d", len(listed))
	}
	read := evalJSON(t, runtime, `console.sessions.read({session_id:"`+id+`"})`).(map[string]any)
	if _, ok := read["header"]; !ok {
		t.Fatalf("read payload has no header: %#v", read)
	}
	deleted := evalJSON(t, runtime, `console.sessions.delete({session_id:"`+id+`"})`).(map[string]any)
	if deleted["deleted"] != true {
		t.Fatal("session was not deleted")
	}
	if header, _ := sessions.Header(id); header != nil {
		t.Fatal("deleted session still visible through Header")
	}
}

func TestConsoleSessionDeleteRefusesActiveSession(t *testing.T) {
	runtime, _ := newConsoleSessionRuntime(t, func(string) bool { return true })
	created := evalJSON(t, runtime, `console.sessions.create({prompt:"active", cwd:"/tmp"})`).(map[string]any)
	session := created["session"].(map[string]any)
	id := session["id"].(string)
	if _, err := runtime.Eval(context.Background(), `console.sessions.delete({session_id:"`+id+`"})`, time.Second); err == nil {
		t.Fatal("active session delete unexpectedly succeeded")
	}
}

func TestConsoleProjectsAddAndList(t *testing.T) {
	runtime, _ := newConsoleSessionRuntime(t, nil)
	directory := t.TempDir()
	added := evalJSON(t, runtime, `console.projects.add({directory:"`+directory+`"})`).(map[string]any)
	if added["path"] != directory {
		t.Fatalf("project path = %#v", added["path"])
	}
	listed := evalJSON(t, runtime, `console.projects.list()`).([]any)
	if len(listed) != 1 {
		t.Fatal("project was not listed")
	}
}
