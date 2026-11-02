// Chrome cookie decryption (the cmux `ChromeCookieDecryptor`, ported to Go
// with stdlib crypto plus golang.org/x/crypto/pbkdf2, both already in the
// module graph — no CGO, no new modules).
//
// macOS Chrome encrypts cookie values AES-128-CBC under a key derived from
// the "Chrome Safe Storage" keychain secret: PBKDF2-SHA1, salt "saltysalt",
// 1003 rounds, IV = 16 spaces. Values carry a "v10"/"v11" prefix. Newer
// stores may hold a GCM payload whose plaintext binds itself to the host
// with a SHA-256 prefix; the GCM key is the same derivation at 32 bytes.
// cmux also tried iteration counts of 1 and base64-encoded keychain
// secrets; the same matrix is attempted in order.
//
// The keychain secret is read by exec'ing the macOS `security` CLI — the
// same consent gate the Swift version hit (the system prompts once for the
// console binary, then allows it). A binary that cannot reach a user
// session (headless daemon over SSH) surfaces a clear error instead of
// hanging: the CLI is killed after a short timeout.
package chromeimport

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const keychainService = "Chrome Safe Storage"

// ErrNoKeychain says the keychain secret could not be read — denied access,
// non-macOS, or a headless session without a keychain to prompt.
var ErrNoKeychain = errors.New("chrome cookie key not available (grant keychain access, or run on a machine with a user session)")

func safeStorageKeys(ctx context.Context) ([]string, error) {
	secret, err := keychainSecret(ctx, keychainService)
	if err != nil {
		return nil, err
	}
	return []string{secret}, nil
}

// keychainSecret runs `security find-generic-password -s <service> -w`.
func keychainSecret(ctx context.Context, service string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-w")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			// 45 = user denied / not found; the CLI writes the reason to stderr.
			return "", fmt.Errorf("%w: %s", ErrNoKeychain, bytes.TrimSpace(exitErr.Stderr))
		}
		return "", fmt.Errorf("%w: %v", ErrNoKeychain, err)
	}
	return string(bytes.TrimSpace(out)), nil
}

// decryptRows turns raw rows into cookies, skipping rows whose value cannot
// be recovered. Keys come from the keychain; tests inject their own.
func decryptRows(rows []rawCookieRow, keys ...string) ([]ChromeCookie, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if len(keys) == 0 {
		fetched, err := safeStorageKeys(context.Background())
		if err != nil {
			return nil, err
		}
		keys = fetched
	}
	now := nowUnixMS()
	out := make([]ChromeCookie, 0, len(rows))
	failedDecrypt, failedPartition, failedExpired := 0, 0, 0
	for _, row := range rows {
		if row.Name == "" {
			// A nameless row is not a cookie Chrome would send.
			continue
		}
		if row.TopFrameSiteKey != "" {
			// CHIPS partitioned cookie: scoped to the third-party embed, not
			// useful (and misleading) in a plain cookie jar.
			failedPartition++
			continue
		}
		value := row.Value
		if value == "" {
			decrypted, ok := decryptCookieValue(row.Encrypted, row.Host, keys)
			if !ok {
				failedDecrypt++
				continue
			}
			value = decrypted
		}
		expires := chromeEpochToUnixMS(row.ExpiresUTC)
		if expires > 0 && expires < now {
			failedExpired++
			continue
		}
		out = append(out, ChromeCookie{
			Domain:      row.Host,
			Path:        row.Path,
			Name:        row.Name,
			Value:       value,
			ExpiresAtMs: expires,
			Secure:      row.Secure,
			HTTPOnly:    row.HTTPOnly,
		})
	}
	if len(out) == 0 && len(rows) > 0 {
		slog.Warn("chrome import: no cookies decrypted",
			"rows", len(rows), "decrypt_failed", failedDecrypt,
			"partitioned", failedPartition, "expired", failedExpired)
	}
	return out, nil
}

// decryptCookieValue tries every derivation profile against one value.
func decryptCookieValue(encrypted []byte, host string, keychainSecrets []string) (string, bool) {
	if len(encrypted) < 4 {
		return "", false
	}
	// The version prefix rides in front of the payload; an untagged payload
	// is tried as-is.
	payload := encrypted
	if encrypted[0] == 'v' && len(encrypted) > 3 {
		payload = encrypted[3:]
	}
	for _, secret := range keychainSecrets {
		for _, material := range keyMaterials(secret) {
			for _, iterations := range []int{1003, 1} {
				if value, ok := tryCBC(payload, material, host, iterations); ok {
					return value, true
				}
				if value, ok := tryGCM(payload, material, host, iterations); ok {
					return value, true
				}
			}
		}
	}
	return "", false
}

// keyMaterials expands one keychain secret into both forms cmux tried: the
// raw string and its base64 decoding, for stores that re-encoded it.
func keyMaterials(secret string) [][]byte {
	raw := bytes.TrimRight([]byte(secret), "\x00")
	materials := [][]byte{raw}
	if decoded, err := base64.StdEncoding.DecodeString(string(raw)); err == nil && len(decoded) > 0 {
		materials = append(materials, decoded)
	}
	return materials
}

var saltySalt = []byte("saltysalt")

// Chris Paget / Chromium's fixed IV: sixteen spaces.
var ivSpaces = bytes.Repeat([]byte{' '}, 16)

func deriveKey(material []byte, iterations, length int) []byte {
	return pbkdf2.Key(material, saltySalt, iterations, length, sha1.New)
}

func tryCBC(payload, material []byte, host string, iterations int) (string, bool) {
	if len(payload) == 0 || len(payload)%16 != 0 {
		return "", false
	}
	block, err := aes.NewCipher(deriveKey(material, iterations, 16))
	if err != nil {
		return "", false
	}
	plain := make([]byte, len(payload))
	cipher.NewCBCDecrypter(block, ivSpaces).CryptBlocks(plain, payload)
	// PKCS7 padding: the tail says how many bytes to drop from the end.
	pad := int(plain[len(plain)-1])
	valid := pad >= 1 && pad <= 16 && pad <= len(plain)
	for i := len(plain) - pad; i < len(plain) && valid; i++ {
		if int(plain[i]) != pad {
			valid = false
		}
	}
	if !valid {
		return "", false
	}
	plaintext := plain[:len(plain)-pad]
	// Chrome's macOS v10 cookie values bind the encrypted bytes to their
	// host key: SHA-256(host_key) precedes the actual cookie value. The Swift
	// importer strips this prefix after both CBC and GCM decryption.
	if hostDigest := sha256.Sum256([]byte(host)); bytes.HasPrefix(plaintext, hostDigest[:]) {
		plaintext = plaintext[sha256.Size:]
	}
	value := string(plaintext)
	if !safePlaintext(value) {
		return "", false
	}
	return value, true
}

func tryGCM(payload, material []byte, host string, iterations int) (string, bool) {
	// 12-byte nonce + ciphertext + 16-byte tag.
	if len(payload) < 12+16+1 {
		return "", false
	}
	block, err := aes.NewCipher(deriveKey(material, iterations, 32))
	if err != nil {
		return "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false
	}
	plain, err := gcm.Open(nil, payload[:12], payload[12:], nil)
	if err != nil {
		return "", false
	}
	// The GCM payload binds itself to the host with a SHA-256 prefix.
	if hostDigest := sha256.Sum256([]byte(host)); bytes.HasPrefix(plain, hostDigest[:]) {
		plain = plain[32:]
	}
	value := string(plain)
	if !safePlaintext(value) {
		return "", false
	}
	return value, true
}

// safePlaintext rejects decrypt-with-the-wrong-key noise: cookie values are
// almost always printable, never starting with control bytes.
func safePlaintext(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 {
		// An empty value decrypts to nothing either way; harmless to keep.
		return true
	}
	for _, r := range runes {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

func nowUnixMS() int64 {
	// Stored as ms; time.Now is fine at this granularity.
	return time.Now().UnixMilli()
}

// chromeEpochToUnixMS converts Chrome's µs since 1601 clock; 0 stays 0
// (session cookie).
func chromeEpochToUnixMS(expiresUTC int64) int64 {
	if expiresUTC <= 0 {
		return 0
	}
	const windowsEpochDeltaSeconds = 11_644_473_600
	seconds := int64(float64(expiresUTC)/1_000_000) - windowsEpochDeltaSeconds
	return seconds * 1_000
}
