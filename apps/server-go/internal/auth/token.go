// API bearer tokens for the QUIC/WebTransport migration (Phase 0).
//
// These are short-lived, HMAC-signed handshake credentials: a client mints
// one over REST (POST /api/auth/token) and presents it before a WebTransport
// session is accepted, where the device id is bound to the QUIC connection.
// They are NOT provider credentials and never touch the LLM layer.
//
// Wire shape: `ct1_<base64url(payload)>.<base64url(sig)>` where the payload
// is `{"v":1,"device":...,"iat":ms,"exp":ms,"jti":...}` and the signature is
// HMAC-SHA256(secret, payloadPart). Base64url keeps tokens safe in both an
// `Authorization: Bearer` header and a `?token=` query string with no
// further encoding.
//
// The HMAC secret is per-install, persisted beside the other credentials
// (dir 0700, file 0600, mirroring the GitHub credential file). Revocations
// live in memory and are pruned by expiry: tokens are handshake-scoped with
// a minutes-scale TTL, so a restart can only extend a revoked token's life
// to its expiry — never beyond it.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// DefaultTokenTTL covers mint-to-connect with room to spare. Tokens are
// validated once, before a WT session is accepted — never per frame — so a
// short TTL costs nothing at runtime.
const DefaultTokenTTL = 5 * time.Minute

// tokenVersion pins the payload schema; bump on any format change and old
// tokens fail closed at validation.
const tokenVersion = 1

// tokenPrefix marks the wire shape for logs and greppability. It carries no
// security meaning.
const tokenPrefix = "ct1_"

// Sentinel validation failures, matched with errors.Is like the provider
// credential errors.
var (
	ErrTokenInvalid = errors.New("API token is invalid.")
	ErrTokenExpired = errors.New("API token has expired.")
	ErrTokenRevoked = errors.New("API token has been revoked.")
)

type tokenPayload struct {
	V      int    `json:"v"`
	Kind   string `json:"kind,omitempty"`
	Device string `json:"device"`
	Iat    int64  `json:"iat"`
	Exp    int64  `json:"exp"`
	Jti    string `json:"jti"`
}

type Claims struct {
	Kind, Device, JTI   string
	IssuedAt, ExpiresAt int64
}

// TokenIssuer mints and validates API bearer tokens. Safe for concurrent use
// from Fiber handlers. The zero value is NOT usable — construct with
// NewTokenIssuer or NewTokenIssuerWithSecret.
type TokenIssuer struct {
	mu      sync.Mutex
	secret  []byte
	revoked map[string]int64 // jti -> exp millis, pruned by expiry
}

// NewTokenIssuer loads the per-install secret, creating and persisting one
// on first run. A storage failure falls back to an ephemeral secret with a
// warning: the server keeps working, and only token survival across restarts
// is lost (tokens are minutes-lived handshake credentials).
func NewTokenIssuerStrict() (*TokenIssuer, error) {
	secret, err := loadOrCreateTokenSecret()
	if err != nil {
		return nil, fmt.Errorf("initialize API token secret: %w", err)
	}
	return &TokenIssuer{secret: secret, revoked: map[string]int64{}}, nil
}

func NewTokenIssuer() *TokenIssuer {
	issuer, err := NewTokenIssuerStrict()
	if err != nil {
		slog.Warn("API token secret unavailable, using an ephemeral secret", "error", err)
		secret := make([]byte, 32)
		if _, randErr := rand.Read(secret); randErr != nil {
			panic(randErr)
		}
		return &TokenIssuer{secret: secret, revoked: map[string]int64{}}
	}
	return issuer
}

// NewTokenIssuerWithSecret builds an issuer around an explicit secret
// (tests, or deployments that manage the secret externally). The secret is
// copied; it must be non-empty.
func NewTokenIssuerWithSecret(secret []byte) *TokenIssuer {
	if len(secret) == 0 {
		panic("API token secret must not be empty")
	}
	return &TokenIssuer{secret: append([]byte(nil), secret...), revoked: map[string]int64{}}
}

// TokenSecretPath resolves the secret file: CONSOLE_TOKEN_SECRET_PATH wins
// outright (tests, unusual deployments), else <storage>/api-token-secret so
// dev installs and CONSOLE_STORAGE_DIR overrides keep separate secrets.
func TokenSecretPath() string {
	if override := os.Getenv("CONSOLE_TOKEN_SECRET_PATH"); override != "" {
		return override
	}
	return filepath.Join(utils.ConsoleStorageDir(), "api-token-secret")
}

func loadOrCreateTokenSecret() ([]byte, error) {
	path := TokenSecretPath()
	if raw, err := os.ReadFile(path); err == nil {
		if secret, decErr := hex.DecodeString(strings.TrimSpace(string(raw))); decErr == nil && len(secret) >= 16 {
			return secret, nil
		}
		// Corrupt or short file: fall through and replace it rather than
		// running with a weak secret.
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// MkdirAll leaves an existing directory's mode alone, and anything
	// looser than 0700 here would weaken the file below it.
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

// Mint issues a token binding deviceID to future WT handshakes. A
// non-positive ttl selects DefaultTokenTTL; a negative ttl mints an
// already-expired token (deterministic expiry tests, never used by callers).
// The returned expiresAt is unix millis, matching the rest of the API.
func (t *TokenIssuer) MintKind(kind, deviceID string, ttl time.Duration) (token string, expiresAt int64, err error) {
	if strings.TrimSpace(kind) == "" {
		return "", 0, ErrTokenInvalid
	}
	return t.mint(kind, deviceID, ttl)
}

func (t *TokenIssuer) Mint(deviceID string, ttl time.Duration) (token string, expiresAt int64, err error) {
	return t.mint("device", deviceID, ttl)
}

func (t *TokenIssuer) mint(kind, deviceID string, ttl time.Duration) (token string, expiresAt int64, err error) {
	if strings.TrimSpace(deviceID) == "" {
		return "", 0, fmt.Errorf("device ID is required.")
	}
	if ttl == 0 {
		ttl = DefaultTokenTTL
	}
	now := utils.NowMillis()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", 0, err
	}
	payload := tokenPayload{
		V:      tokenVersion,
		Kind:   kind,
		Device: deviceID,
		Iat:    now,
		Exp:    now + ttl.Milliseconds(),
		Jti:    hex.EncodeToString(jti),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	t.mu.Lock()
	t.sweepLocked(now)
	t.mu.Unlock()
	return tokenPrefix + body + "." + sig, payload.Exp, nil
}

// Validate checks a presented token and returns the bound device ID. The
// caller (WT accept in Phase 2, the shared middleware) binds it to the
// connection.
func (t *TokenIssuer) Claims(token string) (Claims, error) {
	payload, err := t.parse(token)
	if err != nil {
		return Claims{}, err
	}
	now := utils.NowMillis()
	if now >= payload.Exp {
		return Claims{}, ErrTokenExpired
	}
	t.mu.Lock()
	t.sweepLocked(now)
	_, revoked := t.revoked[payload.Jti]
	t.mu.Unlock()
	if revoked {
		return Claims{}, ErrTokenRevoked
	}
	return Claims{Kind: payload.Kind, Device: payload.Device, JTI: payload.Jti, IssuedAt: payload.Iat, ExpiresAt: payload.Exp}, nil
}

func (t *TokenIssuer) Validate(token string) (string, error) {
	payload, err := t.parse(token)
	if err != nil {
		return "", err
	}
	now := utils.NowMillis()
	if now >= payload.Exp {
		return "", ErrTokenExpired
	}
	t.mu.Lock()
	t.sweepLocked(now)
	_, revoked := t.revoked[payload.Jti]
	t.mu.Unlock()
	if revoked {
		return "", ErrTokenRevoked
	}
	return payload.Device, nil
}

// Revoke invalidates a single token (single-session revocation). The token
// must still verify: only genuinely issued tokens occupy the revocation set,
// which is pruned by expiry so it stays bounded.
func (t *TokenIssuer) Revoke(token string) error {
	payload, err := t.parse(token)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.sweepLocked(utils.NowMillis())
	t.revoked[payload.Jti] = payload.Exp
	t.mu.Unlock()
	return nil
}

// parse decodes and signature-verifies a token without checking expiry or
// revocation. Shared by Validate and Revoke.
func (t *TokenIssuer) parse(token string) (tokenPayload, error) {
	var out tokenPayload
	if !strings.HasPrefix(token, tokenPrefix) {
		return out, ErrTokenInvalid
	}
	body, sig, ok := strings.Cut(strings.TrimSpace(token[len(tokenPrefix):]), ".")
	if !ok || body == "" || sig == "" {
		return out, ErrTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != body {
		return out, ErrTokenInvalid
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, ErrTokenInvalid
	}
	if out.V != tokenVersion || out.Kind == "" || out.Device == "" || out.Jti == "" || out.Exp <= 0 {
		return out, ErrTokenInvalid
	}
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(body))
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return out, ErrTokenInvalid
	}
	if subtle.ConstantTimeCompare(mac.Sum(nil), want) != 1 {
		return out, ErrTokenInvalid
	}
	return out, nil
}

// sweepLocked drops revocations that can no longer validate anyway.
// Caller must hold t.mu.
func (t *TokenIssuer) sweepLocked(now int64) {
	for jti, exp := range t.revoked {
		if now >= exp {
			delete(t.revoked, jti)
		}
	}
}
