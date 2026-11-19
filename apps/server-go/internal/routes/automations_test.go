package routes

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

func newAuthedTestApp(t *testing.T) (*db.DB, string, func(*http.Request) (*http.Response, error), string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONSOLE_STORAGE_DIR", dir)
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)

	issuer := auth.NewTokenIssuerWithSecret([]byte("test-secret-32-bytes-long-enough"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	app, _, shutdown := New(Config{DB: database, TokenIssuer: issuer})
	t.Cleanup(shutdown)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.ShutdownWithTimeout(50 * time.Millisecond) })

	baseURL := "http://" + ln.Addr().String()
	do := func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Authorization", "Bearer "+paired.Token)
		return http.DefaultClient.Do(req)
	}
	return database, paired.Token, do, baseURL
}

func TestAutomationRoutesAndSSE(t *testing.T) {
	database, token, do, baseURL := newAuthedTestApp(t)
	store := automations.New(database.Global())

	sseReq, _ := http.NewRequest(http.MethodGet, baseURL+"/api/automations/stream", nil)
	sseReq.Header.Set("Authorization", "Bearer "+token)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status = %d", sseResp.StatusCode)
	}

	reader := bufio.NewReader(sseResp.Body)
	readEvent := func() (string, string) {
		var ev, data string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("sse read error: %v", err)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if ev != "" || data != "" {
					return ev, data
				}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				ev = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
	}

	ev, data := readEvent()
	if ev != "automations" || data != "[]" {
		t.Fatalf("initial sse = (%q, %q), want (automations, [])", ev, data)
	}

	a, err := store.Create("s1", "check status", automations.KindInterval, 300, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/automations", nil)
	resp, err := do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/automations = %v, %v", resp, err)
	}
	var listOut struct {
		Success bool                     `json:"success"`
		Data    []automations.Automation `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listOut)
	resp.Body.Close()
	if len(listOut.Data) != 1 || listOut.Data[0].ID != a.ID {
		t.Fatalf("list = %+v", listOut)
	}

	req, _ = http.NewRequest(http.MethodGet, baseURL+"/api/sessions/s1/automation", nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/sessions/s1/automation = %v, %v", resp, err)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodPost, baseURL+"/api/automations/"+a.ID+"/pause", nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("pause = %v, %v", resp, err)
	}
	resp.Body.Close()

	ev, data = readEvent()
	if ev != "automation" || !strings.Contains(data, `"type":"paused"`) {
		t.Fatalf("pause sse = (%q, %q)", ev, data)
	}

	req, _ = http.NewRequest(http.MethodPost, baseURL+"/api/automations/"+a.ID+"/resume", nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("resume = %v, %v", resp, err)
	}
	resp.Body.Close()

	ev, data = readEvent()
	if ev != "automation" || !strings.Contains(data, `"type":"resumed"`) {
		t.Fatalf("resume sse = (%q, %q)", ev, data)
	}

	req, _ = http.NewRequest(http.MethodDelete, baseURL+"/api/automations/"+a.ID, nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete = %v, %v", resp, err)
	}
	resp.Body.Close()

	ev, data = readEvent()
	if ev != "automation" || !strings.Contains(data, `"type":"deleted"`) {
		t.Fatalf("delete sse = (%q, %q)", ev, data)
	}

	_ = httptest.NewRequest
}
