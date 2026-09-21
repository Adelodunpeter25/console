// Antigravity OAuth credential storage and refresh. Port of
// apps/server/providers/src/auth/token-store.ts and token-refresh.ts.
package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// OAuthCredential is the canonical shape written back after a refresh.
// Reading tolerates the CLI's older field-name variants (see ParseCredential).
type OAuthCredential struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ProjectID    string `json:"projectId"`
	Email        string `json:"email,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

// ParsedCredential is the validated in-memory credential.
type ParsedCredential struct {
	AccessToken  string
	ProjectID    string
	RefreshToken string
	ExpiresAtMs  int64
	Email        string
}

// CredentialPath mirrors token-store.ts: ANTIGRAVITY_CREDENTIALS_PATH
// override, otherwise ~/.console/antigravity-creds.json.
func CredentialPath() string {
	return shared.CredentialPath("ANTIGRAVITY_CREDENTIALS_PATH", "antigravity-creds.json")
}

func numberField(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := shared.NumberField(m, k); ok {
			return v, true
		}
	}
	return 0, false
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// normalizeExpiryMs converts epoch seconds or ms to ms, mirroring
// normalizeExpiryMs in token-store.ts.
func normalizeExpiryMs(v float64, ok bool) int64 {
	if !ok || v <= 0 {
		return 0
	}
	if v < 10_000_000_000 {
		return int64(v * 1000)
	}
	return int64(v)
}

// ParseCredential validates a raw credential JSON blob, tolerating the
// Antigravity CLI's field-name variants across versions.
func ParseCredential(raw map[string]any) (ParsedCredential, error) {
	accessToken := stringField(raw, "access_token", "token")
	projectID := stringField(raw, "projectId", "project_id")
	if accessToken == "" {
		return ParsedCredential{}, fmt.Errorf("Missing access token in OAuth credential. Please login again.")
	}
	if projectID == "" {
		return ParsedCredential{}, fmt.Errorf("Missing projectId in OAuth credential. Please login again.")
	}
	expiry, ok := numberField(raw, "expiresAt", "expires", "expiry_date")
	expiresAtMs := normalizeExpiryMs(expiry, ok)
	return ParsedCredential{
		AccessToken:  accessToken,
		ProjectID:    projectID,
		RefreshToken: stringField(raw, "refreshToken", "refresh", "refresh_token"),
		ExpiresAtMs:  expiresAtMs,
		Email:        stringField(raw, "email"),
	}, nil
}

// LoadCredential reads and parses the credential file.
func LoadCredential() (ParsedCredential, error) {
	raw, err := os.ReadFile(CredentialPath())
	if err != nil {
		return ParsedCredential{}, err
	}
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		return ParsedCredential{}, fmt.Errorf("Invalid JSON in OAuth credential file: %s", CredentialPath())
	}
	return ParseCredential(m)
}

// SaveCredential writes the credential file (0600 dir 0700).
func SaveCredential(cred OAuthCredential) error {
	return shared.SaveCredentialFile(CredentialPath(), cred)
}

// CredentialExists reports whether a login exists.
func CredentialExists() bool {
	return shared.CredentialFileExists(CredentialPath())
}

var (
	inflightMu  sync.Mutex
	inflight    map[string]chan struct{}
	inflightOut map[string]ParsedCredential
	inflightErr map[string]error
)

func init() {
	inflight = map[string]chan struct{}{}
	inflightOut = map[string]ParsedCredential{}
	inflightErr = map[string]error{}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func doRefresh(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	if cred.RefreshToken == "" {
		return ParsedCredential{}, fmt.Errorf("OAuth token expired and no refresh_token available. Please login again.")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	body := url.Values{
		"client_id":     {ClientID()},
		"client_secret": {ClientSecret()},
		"refresh_token": {cred.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequest("POST", OAuthTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return ParsedCredential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return ParsedCredential{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ParsedCredential{}, shared.TokenError("Antigravity", resp.StatusCode, string(raw))
	}
	var data tokenResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return ParsedCredential{}, fmt.Errorf("decode Antigravity token response: %w", err)
	}
	expiresAtMs := time.Now().UnixMilli() + data.ExpiresIn*1000
	updated := OAuthCredential{
		Token: data.AccessToken, RefreshToken: cred.RefreshToken,
		ProjectID: cred.ProjectID, Email: cred.Email, ExpiresAt: expiresAtMs,
	}
	if err := SaveCredential(updated); err != nil {
		return ParsedCredential{}, err
	}
	return ParsedCredential{
		AccessToken: data.AccessToken, ProjectID: cred.ProjectID,
		RefreshToken: cred.RefreshToken, ExpiresAtMs: expiresAtMs, Email: cred.Email,
	}, nil
}

// RefreshIfNeeded refreshes an expiring credential (60s skew), deduping
// concurrent refreshes for the same credential (keyed by refresh token).
func RefreshIfNeeded(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	if cred.ExpiresAtMs == 0 || time.Now().UnixMilli()+RefreshSkewMs < cred.ExpiresAtMs {
		return cred, nil
	}
	key := cred.RefreshToken
	inflightMu.Lock()
	if ch, ok := inflight[key]; ok {
		inflightMu.Unlock()
		<-ch
		inflightMu.Lock()
		out, outErr := inflightOut[key], inflightErr[key]
		inflightMu.Unlock()
		return out, outErr
	}
	ch := make(chan struct{})
	inflight[key] = ch
	inflightMu.Unlock()

	out, err := doRefresh(client, cred)

	inflightMu.Lock()
	inflightOut[key], inflightErr[key] = out, err
	delete(inflight, key)
	inflightMu.Unlock()
	close(ch)
	return out, err
}
