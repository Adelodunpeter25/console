package routes

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/MoeForge/moego/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"os"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
)

func TestWebhookRouteIsPublicAndVerifiesAuth(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	app, runSvc, shutdown := New(Config{DB: database})
	defer shutdown()

	store := webhooks.New(database.Global())
	runSvc.SetWebhooks(store)

	sub, err := store.Create("missing-session", "ci", "check build", "top-secret", map[string]any{"status": "failed"}, true, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"status":"failed","job":"android"}`)

	// 1. Unknown subscription returns 404 without needing a device token.
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/ci/sub_unknown?token=top-secret", bytes.NewReader(body))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown sub status = %d, want 404", resp.StatusCode)
	}

	// 2. Wrong secret returns 401.
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/ci/"+sub.ID+"?token=wrong", bytes.NewReader(body))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want 401", resp.StatusCode)
	}

	// 3. Valid HMAC signature reaches DeliverWebhook (session doesn't exist so
	// AfterFire marks it error, and route returns 200 with matched=0).
	mac := hmac.New(sha256.New, []byte("top-secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/ci/"+sub.ID, bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid hmac status = %d, want 200", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["success"] != true {
		t.Fatalf("response = %#v", out)
	}
	got, _ := store.Get(sub.ID)
	if got.Status != webhooks.StatusError {
		t.Fatalf("missing session should mark sub error, got %q", got.Status)
	}
}

func TestWebhookBaseURLReadsOpenTunnelAlongsideIroh(t *testing.T) {
	dir := t.TempDir()
	if err := tunnel.SetBackend(dir, tunnel.BackendIroh); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tunnel.json"), []byte(`{"id":"t1","hostname":"hooks.example.test","certificateExpiry":"2030-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := webhookBaseURLFromDir(dir); got != "https://hooks.example.test" {
		t.Fatalf("webhookBaseURLFromDir = %q, want https://hooks.example.test", got)
	}
}

func TestWebhookManagementRoutesAndSSE(t *testing.T) {
	_, token, do, baseURL := newAuthedTestApp(t)

	sseReq, _ := http.NewRequest(http.MethodGet, baseURL+"/api/webhooks/stream", nil)
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
	if ev != "webhooks" || data != "[]" {
		t.Fatalf("initial sse = (%q, %q), want (webhooks, [])", ev, data)
	}

	createBody := []byte(`{"sessionId":"s1","topic":"ci","prompt":"fix build","once":true}`)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/webhooks", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/webhooks = %v, %v", resp, err)
	}
	var created struct {
		Success bool        `json:"success"`
		Data    webhookView `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Data.ID == "" || created.Data.Topic != "ci" {
		t.Fatalf("created = %+v", created)
	}

	ev, data = readEvent()
	if ev != "webhook" || !strings.Contains(data, `"type":"created"`) {
		t.Fatalf("create sse = (%q, %q)", ev, data)
	}

	req, _ = http.NewRequest(http.MethodGet, baseURL+"/api/webhooks?sessionId=s1", nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/webhooks = %v, %v", resp, err)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, baseURL+"/api/webhooks/"+created.Data.ID, nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/webhooks/:id = %v, %v", resp, err)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodDelete, baseURL+"/api/webhooks/"+created.Data.ID, nil)
	resp, err = do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/webhooks/:id = %v, %v", resp, err)
	}
	resp.Body.Close()

	ev, data = readEvent()
	if ev != "webhook" || !strings.Contains(data, `"type":"deleted"`) {
		t.Fatalf("delete sse = (%q, %q)", ev, data)
	}
}
