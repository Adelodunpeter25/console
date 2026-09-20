// Shared OAuth helpers for subscription providers (codex, claude, ...).
// Ports the generic parts of apps/server/providers/src/auth/token-store.ts
// and the PKCE/token-POST helpers duplicated across provider oauth modules.
package shared

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DecodeJWTPayload decodes the middle JWT segment without verification.
func DecodeJWTPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// GeneratePKCE creates a verifier/challenge pair (S256).
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// TokenError formats an OAuth token endpoint failure, preferring a JSON
// error_description/error/message field when the body is JSON.
func TokenError(provider string, status int, body string) error {
	detail := strings.TrimSpace(body)
	if detail != "" {
		var parsed struct {
			Error       any `json:"error"`
			Description any `json:"error_description"`
			Message     any `json:"message"`
		}
		if err := json.Unmarshal([]byte(detail), &parsed); err == nil {
			for _, v := range []any{parsed.Description, parsed.Error, parsed.Message} {
				if s, ok := v.(string); ok && s != "" {
					detail = s
					break
				}
			}
		}
	}
	if detail == "" {
		detail = "unknown error"
	}
	return fmt.Errorf("%s OAuth request failed (%d): %s", provider, status, detail)
}

// PostTokenForm POSTs a form to a token endpoint and returns the raw body.
func PostTokenForm(client *http.Client, tokenURL string, body url.Values) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return raw, nil
}

// CredentialPath resolves a credential file: env override or ~/.console/<name>.
func CredentialPath(envKey, fileName string) string {
	if p := os.Getenv(envKey); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".console", fileName)
}

// SaveCredentialFile writes JSON credentials (dir 0700, file 0600).
func SaveCredentialFile(path string, cred any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// CredentialFileExists reports whether the credential file exists.
func CredentialFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
