// Development certificate generation for `console start --wt`.
//
// The assertions here are mostly about constraints imposed from the *outside*
// by wtransport's `ServerHashVerification`, which is what the desktop and
// mobile clients use to pin a dev certificate. Those constraints reject RSA
// keys and validity past 14 days, and a certificate that violates them still
// looks perfectly valid to every other tool in the box — so the only way to
// catch a regression is to assert on them here.
package tests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func parseDevCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("%s is not a PEM certificate", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestEnsureDevCertProducesAPinnableCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "wt-dev.crt")
	keyPath := filepath.Join(dir, "wt-dev.key")

	cert, err := wt.EnsureDevCert(certPath, keyPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.Generated {
		t.Fatal("first call must generate")
	}

	parsed := parseDevCert(t, certPath)

	// wtransport's pinning refuses anything but EC P-256.
	key, ok := parsed.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("pinned certs must be ECDSA, got %T", parsed.PublicKeyAlgorithm)
	}
	if key.Curve.Params().Name != "P-256" {
		t.Fatalf("curve must be P-256, got %s", key.Curve.Params().Name)
	}

	// ...and refuses validity past 14 days.
	if validity := parsed.NotAfter.Sub(parsed.NotBefore); validity > 14*24*time.Hour {
		t.Fatalf("validity %s exceeds the 14-day pinning limit", validity)
	}

	// NotBefore is backdated so a slightly-behind client still accepts it.
	if parsed.NotBefore.After(time.Now()) {
		t.Fatal("NotBefore must be backdated, not in the future")
	}

	sum := sha256.Sum256(parsed.Raw)
	if want := hex.EncodeToString(sum[:]); cert.Fingerprint != want {
		t.Fatalf("fingerprint is the SHA-256 of the DER cert:\n got %s\nwant %s", cert.Fingerprint, want)
	}
}

func TestEnsureDevCertCoversDialableNames(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "wt-dev.crt")

	if _, err := wt.EnsureDevCert(certPath, filepath.Join(dir, "wt-dev.key"), []string{"console.local"}); err != nil {
		t.Fatal(err)
	}
	parsed := parseDevCert(t, certPath)

	dnsNames := map[string]bool{}
	for _, n := range parsed.DNSNames {
		dnsNames[n] = true
	}
	ips := map[string]bool{}
	for _, ip := range parsed.IPAddresses {
		ips[ip.String()] = true
	}

	// The emulator reaches the host loopback at this fixed alias; without it
	// a mobile client fails host verification against the dev server.
	if !ips["10.0.2.2"] {
		t.Fatalf("emulator host alias missing from SANs: %v", parsed.IPAddresses)
	}
	if !dnsNames["localhost"] {
		t.Fatalf("localhost missing from SANs: %v", parsed.DNSNames)
	}
	if !ips["127.0.0.1"] {
		t.Fatalf("loopback missing from SANs: %v", parsed.IPAddresses)
	}
	if !dnsNames["console.local"] {
		t.Fatalf("caller-supplied host missing from SANs: %v", parsed.DNSNames)
	}
}

func TestEnsureDevCertReusesAValidCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "wt-dev.crt")

	first, err := wt.EnsureDevCert(certPath, filepath.Join(dir, "wt-dev.key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := wt.EnsureDevCert(certPath, filepath.Join(dir, "wt-dev.key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	if second.Generated {
		t.Fatal("a valid certificate must be reused, not regenerated")
	}
	if second.Fingerprint != first.Fingerprint {
		t.Fatalf("reused fingerprint changed:\n first %s\nsecond %s", first.Fingerprint, second.Fingerprint)
	}
}

func TestEnsureDevCertReplacesExpiredAndUnusableCertificates(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "wt-dev.crt")
	keyPath := filepath.Join(dir, "wt-dev.key")

	if _, err := wt.EnsureDevCert(certPath, keyPath, nil); err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}

	// Every one of these must be replaced rather than accepted: an expired
	// cert stops pinning, and an RSA one was never pinnable in the first place.
	cases := map[string][]byte{
		"expired":    mustGenerateWithValidity(t, 0, -48*time.Hour),
		"rsa":        mustGenerateRSA(t),
		"garbage":    []byte("not a certificate at all"),
		"empty-file": {},
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(certPath, contents, 0o644); err != nil {
				t.Fatal(err)
			}
			cert, err := wt.EnsureDevCert(certPath, keyPath, nil)
			if err != nil {
				t.Fatalf("%s must be replaced, not fatal: %v", name, err)
			}
			if !cert.Generated {
				t.Fatalf("%s certificate was accepted", name)
			}
			restored, err := os.ReadFile(certPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(restored) == string(contents) {
				t.Fatalf("%s certificate was left in place", name)
			}
		})
	}

	// Sanity check that the replacement is a working certificate, not merely
	// different bytes.
	if parseDevCert(t, certPath).PublicKeyAlgorithm != x509.ECDSA {
		t.Fatal("replacement cert should be ECDSA")
	}
	_ = good
}

func TestEnsureDevCertKeyIsNotWorldReadable(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "wt-dev.key")
	if _, err := wt.EnsureDevCert(filepath.Join(dir, "wt-dev.crt"), keyPath, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("private key must be 0600, got %o", perm)
	}
}

func TestEnsureDevCertCreatesMissingDirectories(t *testing.T) {
	// `console start` can be the first thing a user ever runs, so the
	// certificate must not depend on the console directory already existing.
	dir := filepath.Join(t.TempDir(), "nested", "console-dev")
	if _, err := wt.EnsureDevCert(filepath.Join(dir, "wt-dev.crt"), filepath.Join(dir, "wt-dev.key"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wt-dev.crt")); err != nil {
		t.Fatal(err)
	}
}

// mustGenerateWithValidity writes a certificate whose validity window is
// offset by notBefore/notAfter, producing an already-expired one.
func mustGenerateWithValidity(t *testing.T, notBefore, notAfter time.Duration) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(notBefore),
		NotAfter:     time.Now().Add(notAfter),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// mustGenerateRSA writes a structurally fine RSA certificate — exactly the kind
// that looks valid to openssl but that client-side pinning refuses.
func mustGenerateRSA(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}