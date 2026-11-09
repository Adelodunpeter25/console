// Cloudflare speech-to-text: credential storage, the upstream client, and the
// /api/stt HTTP surface.
package stt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/stt"
)

// useTempStore isolates the credential file and clears the env pair so a
// developer's real Cloudflare account cannot change a test result.
func useTempStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stt.json")
	t.Setenv("CONSOLE_STT_CONFIG_PATH", path)
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	return path
}

func useStoredConfig(t *testing.T) {
	t.Helper()
	useTempStore(t)
	if err := stt.Set("acct-123", "token-abc"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := useTempStore(t)

	if err := stt.Set("acct-123", "token-abc"); err != nil {
		t.Fatal(err)
	}
	stored, err := stt.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccountID != "acct-123" || stored.APIToken != "token-abc" {
		t.Fatalf("round trip failed: %+v", stored)
	}
	if stored.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file must be 0600, got %o", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config directory must be 0700, got %o", perm)
	}

	status, err := stt.CurrentStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Source != stt.SourceStored || status.AccountID != "acct-123" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Model != stt.Model || status.MaxAudioBytes != stt.MaxAudioBytes {
		t.Fatalf("static fields missing: %+v", status)
	}

	if err := stt.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Clear should remove the file, stat err=%v", err)
	}
	status, err = stt.CurrentStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured {
		t.Fatalf("status should be unconfigured after Clear: %+v", status)
	}
}

func TestBlankFieldsAreRejected(t *testing.T) {
	useTempStore(t)

	if err := stt.Set("", "token"); !errors.Is(err, stt.ErrEmptyAccountID) {
		t.Fatalf("expected ErrEmptyAccountID, got %v", err)
	}
	if err := stt.Set("acct", " \t\n"); !errors.Is(err, stt.ErrEmptyAPIToken) {
		t.Fatalf("expected ErrEmptyAPIToken, got %v", err)
	}
	stored, err := stt.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccountID != "" || stored.APIToken != "" {
		t.Fatalf("a rejected pair must not be stored: %+v", stored)
	}
}

func TestEnvPairOverridesStored(t *testing.T) {
	useStoredConfig(t)
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "env-acct")
	t.Setenv("CLOUDFLARE_API_TOKEN", "env-token")

	status, err := stt.CurrentStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Source != stt.SourceEnv || status.AccountID != "env-acct" {
		t.Fatalf("env should win: %+v", status)
	}
	creds, err := stt.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIToken != "env-token" || creds.Source != stt.SourceEnv {
		t.Fatalf("env pair not resolved: %+v", creds)
	}

	// A partial env pair must not shadow a working stored config, matching
	// the provider-key convention.
	t.Setenv("CLOUDFLARE_API_TOKEN", "   ")
	creds, err = stt.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIToken != "token-abc" || creds.Source != stt.SourceStored {
		t.Fatalf("partial env should fall through to stored: %+v", creds)
	}
}

func TestTranscribeSendsCredentialAndReturnsText(t *testing.T) {
	useStoredConfig(t)

	audio := []byte("fake-wave-bytes")
	var gotPath, gotAuth, gotType string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"result":{"text":"  hello world  "},"errors":[],"messages":[]}`)
	}))
	defer upstream.Close()

	text, err := stt.NewClient(upstream.URL).Transcribe(context.Background(), audio, "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q", text)
	}
	if want := "/accounts/acct-123/ai/run/" + stt.Model; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
	if gotAuth != "Bearer token-abc" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotType != "audio/wav" {
		t.Fatalf("content type = %q", gotType)
	}
	if !bytes.Equal(gotBody, audio) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestTranscribeFallsBackToOctetStream(t *testing.T) {
	useStoredConfig(t)

	var gotType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, `{"success":true,"result":{"text":"ok"}}`)
	}))
	defer upstream.Close()

	if _, err := stt.NewClient(upstream.URL).Transcribe(context.Background(), []byte("x"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if gotType != "application/octet-stream" {
		t.Fatalf("content type = %q", gotType)
	}
}

func TestTranscribeRefusesWithoutConfig(t *testing.T) {
	useTempStore(t)

	// The configuration check must happen before the network call, so an
	// unconfigured server reports the fixable problem instead of an opaque
	// connection error.
	_, err := stt.NewClient("http://127.0.0.1:1").Transcribe(context.Background(), []byte("x"), "audio/wav")
	if !errors.Is(err, stt.ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestTranscribeMapsAuthFailure(t *testing.T) {
	useStoredConfig(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"message":"Authentication error"}]}`)
	}))
	defer upstream.Close()

	_, err := stt.NewClient(upstream.URL).Transcribe(context.Background(), []byte("x"), "audio/wav")
	if err == nil || !strings.Contains(err.Error(), "rejected the API token") {
		t.Fatalf("expected a named auth error, got %v", err)
	}
	if strings.Contains(err.Error(), "token-abc") {
		t.Fatalf("the token leaked into the error: %v", err)
	}
}

func TestTranscribeRejectsOversizedAudioBeforeUploading(t *testing.T) {
	useStoredConfig(t)

	_, err := stt.NewClient("http://127.0.0.1:1").Transcribe(
		context.Background(), make([]byte, stt.MaxAudioBytes+1), "audio/wav",
	)
	if !errors.Is(err, stt.ErrAudioTooLarge) {
		t.Fatalf("expected ErrAudioTooLarge, got %v", err)
	}
}

// --- HTTP surface ---------------------------------------------------------

func sttApp(t *testing.T, client *stt.Client) *fiber.App {
	t.Helper()
	// Match the server's headroom for a near-limit multipart upload.
	app := fiber.New(fiber.Config{BodyLimit: 8 << 20})
	routes.RegisterSttRoutes(app, client)
	return app
}

func request(t *testing.T, app *fiber.App, method, path, body, contentType string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}

type sttStatusPayload struct {
	Configured    *bool   `json:"configured"`
	Source        *string `json:"source"`
	AccountID     *string `json:"accountId"`
	Model         string  `json:"model"`
	UpdatedAt     *string `json:"updatedAt"`
	MaxAudioBytes uint32  `json:"maxAudioBytes"`
}

func decodeStatus(t *testing.T, body string) sttStatusPayload {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", body)
	}
	var status sttStatusPayload
	if err := json.Unmarshal(envelope.Data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func decodeConfig(t *testing.T, body string) (sttStatusPayload, bool) {
	t.Helper()
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Status  sttStatusPayload `json:"status"`
			Removed bool             `json:"removed"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", body)
	}
	return envelope.Data.Status, envelope.Data.Removed
}

func TestSttConfigRoutes(t *testing.T) {
	useTempStore(t)
	app := sttApp(t, stt.NewClient("http://127.0.0.1:1"))

	// Before: not configured, and `configured` stays explicitly present.
	code, body := request(t, app, "GET", "/api/stt/status", "", "")
	if code != http.StatusOK {
		t.Fatalf("status: %d %s", code, body)
	}
	status := decodeStatus(t, body)
	if status.Configured == nil || *status.Configured {
		t.Fatalf("configured should be explicit false: %s", body)
	}
	if status.Model != stt.Model || status.MaxAudioBytes != stt.MaxAudioBytes {
		t.Fatalf("static fields missing: %s", body)
	}

	// Set both halves.
	code, body = request(t, app, "PUT", "/api/stt/config",
		`{"accountId":"acct-123","apiToken":"token-abc"}`, "application/json")
	if code != http.StatusOK {
		t.Fatalf("set: %d %s", code, body)
	}
	status, removed := decodeConfig(t, body)
	if removed {
		t.Fatalf("PUT must not report removed: %s", body)
	}
	if status.Configured == nil || !*status.Configured || status.Source == nil || *status.Source != stt.SourceStored {
		t.Fatalf("set should report configured/stored: %s", body)
	}
	if status.AccountID == nil || *status.AccountID != "acct-123" {
		t.Fatalf("set should report the account: %s", body)
	}
	if status.UpdatedAt == nil || *status.UpdatedAt == "" {
		t.Fatalf("stored status should include updatedAt: %s", body)
	}
	if strings.Contains(body, "token-abc") {
		t.Fatalf("the token leaked into the response: %s", body)
	}

	// Read-back.
	_, body = request(t, app, "GET", "/api/stt/status", "", "")
	status = decodeStatus(t, body)
	if status.Configured == nil || !*status.Configured {
		t.Fatalf("GET should report configured: %s", body)
	}
	if strings.Contains(body, "token-abc") {
		t.Fatalf("GET leaked the token: %s", body)
	}

	// Clear.
	code, body = request(t, app, "DELETE", "/api/stt/config", "", "")
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, body)
	}
	status, removed = decodeConfig(t, body)
	if !removed {
		t.Fatalf("DELETE should report removed: %s", body)
	}
	if status.Configured == nil || *status.Configured {
		t.Fatalf("delete should report configured=false: %s", body)
	}
}

func TestSttConfigRejectsBlankFields(t *testing.T) {
	useTempStore(t)
	app := sttApp(t, stt.NewClient("http://127.0.0.1:1"))

	for _, body := range []string{
		`{"accountId":"","apiToken":"token"}`,
		`{"accountId":"acct","apiToken":"   "}`,
	} {
		code, response := request(t, app, "PUT", "/api/stt/config", body, "application/json")
		if code != http.StatusBadRequest {
			t.Fatalf("blank field should be 400, got %d %s", code, response)
		}
	}
}

func TestSttTranscribeRoute(t *testing.T) {
	useStoredConfig(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"result":{"text":"hello from audio"},"errors":[]}`)
	}))
	defer upstream.Close()
	app := sttApp(t, stt.NewClient(upstream.URL))

	body, contentType := multipartAudio(t, "note.wav", "audio/wav", []byte("fake audio"))
	req := httptest.NewRequest("POST", "/api/stt/transcribe", body)
	req.Header.Set("Content-Type", contentType)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("transcribe: %d %s", resp.StatusCode, raw)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Text  string `json:"text"`
			Model string `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if envelope.Data.Text != "hello from audio" || envelope.Data.Model != stt.Model {
		t.Fatalf("unexpected transcript: %s", raw)
	}
}

func TestSttTranscribeRouteRequiresConfig(t *testing.T) {
	useTempStore(t)
	app := sttApp(t, stt.NewClient("http://127.0.0.1:1"))

	body, contentType := multipartAudio(t, "note.wav", "audio/wav", []byte("fake audio"))
	req := httptest.NewRequest("POST", "/api/stt/transcribe", body)
	req.Header.Set("Content-Type", contentType)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unconfigured transcribe should be 400, got %d %s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "not configured") {
		t.Fatalf("error should name the missing config: %s", raw)
	}
}

func TestSttTranscribeRouteRequiresAudio(t *testing.T) {
	useTempStore(t)
	app := sttApp(t, stt.NewClient("http://127.0.0.1:1"))

	code, body := request(t, app, "POST", "/api/stt/transcribe", "", "")
	if code != http.StatusBadRequest {
		t.Fatalf("missing audio should be 400, got %d %s", code, body)
	}
}

func TestSttTranscribeRouteRejectsOversizedAudio(t *testing.T) {
	useStoredConfig(t)
	app := sttApp(t, stt.NewClient("http://127.0.0.1:1"))

	body, contentType := multipartAudio(t, "big.wav", "audio/wav", make([]byte, stt.MaxAudioBytes+1))
	req := httptest.NewRequest("POST", "/api/stt/transcribe", body)
	req.Header.Set("Content-Type", contentType)
	resp, err := app.Test(req, 20000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized audio should be 413, got %d %s", resp.StatusCode, raw)
	}
}

func multipartAudio(t *testing.T, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="audio"; filename="%s"`, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, writer.FormDataContentType()
}
