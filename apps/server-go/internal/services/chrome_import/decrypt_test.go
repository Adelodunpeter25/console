// Decryption tests. Fixtures are round-trips of the documented schemes with
// a fixed keychain secret — the same bytes a real Chrome store holds.
package chromeimport

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const fixtureSecret = "peanuts"

func fixtureKey(material string, length int) []byte {
	return pbkdf2.Key([]byte(material), []byte("saltysalt"), 1003, length, sha1.New)
}

// encCBC builds a "v10"-prefixed AES-128-CBC payload like Chrome's classic
// macOS store: PBKDF2-SHA1 ("saltysalt", 1003 rounds), IV = 16 spaces,
// and SHA-256(host) prepended inside the encrypted plaintext.
func encCBC(key []byte, host, value string) []byte {
	digest := sha256.Sum256([]byte(host))
	plaintext := append(digest[:], []byte(value)...)
	pad := 16 - len(plaintext)%16
	for i := 0; i < pad; i++ {
		plaintext = append(plaintext, byte(pad))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	out := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, bytes.Repeat([]byte{' '}, 16)).CryptBlocks(out, plaintext)
	return append([]byte("v10"), out...)
}

// encGCM builds a GCM payload like newer stores: nonce then ciphertext+tag,
// with the host-bound SHA-256 prefix inside the plaintext.
func encGCM(key []byte, host, value string) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256([]byte(host))
	plain := digest[:]
	plain = append(plain, []byte(value)...)
	nonce := []byte("fixed12-byte") // exactly 12 bytes
	sealed := gcm.Seal(nil, nonce, plain, nil)
	body := append(append([]byte{}, nonce...), sealed...)
	return append([]byte("v10"), body...)
}

// chromeEpochUS converts a time offset into Chrome's µs-since-1601 clock.
func chromeEpochUS(offset time.Duration) int64 {
	const delta = 11_644_473_600
	ms := time.Now().Add(offset).UnixMilli()
	return (ms/1000 + delta) * 1_000_000
}

func TestDecryptCBC(t *testing.T) {
	key := fixtureKey(fixtureSecret, 16)
	value := "session-token-value"
	encrypted := encCBC(key, ".example.com", value)
	got, ok := decryptCookieValue(encrypted, ".example.com", []string{fixtureSecret})
	if !ok || got != value {
		t.Fatalf("value = %q (ok=%v), want %q", got, ok, value)
	}

	if _, ok := decryptCookieValue(encrypted, ".example.com", []string{"wrong-secret"}); ok {
		t.Fatal("a wrong key must not produce a value")
	}
}

func TestDecryptGCM(t *testing.T) {
	key := fixtureKey(fixtureSecret, 32)
	value := "gcm-cookie-value"
	encrypted := encGCM(key, "host.com", value)
	got, ok := decryptCookieValue(encrypted, "host.com", []string{fixtureSecret})
	if !ok || got != value {
		t.Fatalf("value = %q (ok=%v), want %q", got, ok, value)
	}
}

func TestDecryptRowsFiltering(t *testing.T) {
	encrypted := encCBC(fixtureKey(fixtureSecret, 16), "b.com", "secret-1")
	live := chromeEpochUS(24 * time.Hour)
	past := chromeEpochUS(-1 * time.Hour)

	rows := []rawCookieRow{
		{Host: "a.com", Name: "sid", Value: "plain-value", Path: "/", ExpiresUTC: live},
		{Host: "b.com", Name: "tok", Encrypted: encrypted, Path: "/", Secure: true, HTTPOnly: true, ExpiresUTC: live},
		{Host: "c.com", Name: "old", Encrypted: encrypted, Path: "/", ExpiresUTC: past},         // expired
		{Host: "d.com", Name: "pip", Encrypted: encrypted, Path: "/", TopFrameSiteKey: "e.com"}, // CHIPS
		{Host: "e.com", Name: "", Value: "nameless", Path: "/"},                                 // no name
		{Host: "f.com", Name: "void", Value: "", Encrypted: []byte("nonempty"), Path: "/"},      // tombstone fails decrypt
	}

	cookies, err := decryptRows(rows, fixtureSecret)
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 2 {
		t.Fatalf("cookies = %+v, want the plain and the encrypted live rows", cookies)
	}
	if cookies[0].Value != "plain-value" || cookies[0].ExpiresAtMs == 0 {
		t.Fatalf("sid = %+v", cookies[0])
	}
	if cookies[1].Value != "secret-1" || !cookies[1].HTTPOnly || !cookies[1].Secure {
		t.Fatalf("tok = %+v", cookies[1])
	}
}
