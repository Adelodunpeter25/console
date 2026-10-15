package wt

// Self-signed development certificate generation.
//
// This is the Go equivalent of the `openssl` incantation the Makefile used to
// run, and it exists so `console start --wt` can serve WebTransport without the
// user having to think about certificates at all. Doing it in Go rather than
// shelling out fixes three things the shell version could not:
//
//   - no `openssl` dependency (the Makefile had to detect it and bail),
//   - SANs are a struct field, so the `-extensions` typo that silently produced
//     a cert with no subjectAltName is not expressible,
//   - no temporary openssl config file or shell quoting.
//
// The constraints below are not stylistic — they come from wtransport's
// `ServerHashVerification`, which the desktop and mobile clients use to pin a
// dev certificate. It rejects RSA keys and validity longer than 14 days, so a
// "normal" certificate silently fails to pin.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	// devCertValidity is 13 days: wtransport's cert pinning refuses anything
	// past 14, and the shorter window also bounds the blast radius of a
	// generated key that outlives the machine it was made on.
	devCertValidity = 13 * 24 * time.Hour

	// renewBefore triggers regeneration while the current cert is still
	// valid, so a daemon left running across a renewal does not silently
	// start failing to pin.
	renewBefore = 24 * time.Hour

	// devCertClockSkew backdates NotBefore so a client whose clock is a
	// little behind the server still accepts the cert.
	devCertClockSkew = time.Hour

	// emulatorHostAlias is the Android emulator's fixed alias for the host
	// machine's loopback. It is a documented AVD constant rather than an
	// address on this machine, so it cannot come from interface scanning —
	// but it is how every emulator reaches a dev server, and without it in
	// the SAN list a mobile client fails host verification.
	emulatorHostAlias = "10.0.2.2"
)

// DevCert describes a generated development certificate.
type DevCert struct {
	CertPath    string
	KeyPath     string
	// Fingerprint is the lowercase hex SHA-256 of the DER certificate —
	// the exact value a client passes as `CONSOLE_WT_TLS=pin:<fingerprint>`.
	Fingerprint string
	// Generated reports whether this call created the pair (as opposed to
	// finding a usable one already on disk).
	Generated bool
}

// EnsureDevCert returns a usable dev certificate at certPath/keyPath,
// generating one when it is missing, expired, or close enough to expiry that
// it should be replaced.
//
// extraHosts adds subjectAltNames beyond the ones always included (localhost,
// the loopback addresses, and the emulator host alias). Callers pass the
// addresses they are actually serving on.
func EnsureDevCert(certPath, keyPath string, extraHosts []string) (DevCert, error) {
	cert := DevCert{CertPath: certPath, KeyPath: keyPath}

	if existing, ok := loadUsableDevCert(certPath); ok {
		cert.Fingerprint = existing
		return cert, nil
	}

	hosts, err := devCertHosts(extraHosts)
	if err != nil {
		return DevCert{}, err
	}

	fingerprint, err := writeDevCert(certPath, keyPath, hosts)
	if err != nil {
		return DevCert{}, err
	}
	cert.Fingerprint = fingerprint
	cert.Generated = true
	return cert, nil
}

// loadUsableDevCert returns the fingerprint of an existing certificate when it
// can still be used, and ok=false when it is missing, unreadable, not a
// certificate we generated, or close to expiry.
//
// A malformed or foreign certificate counts as unusable rather than an error:
// replacing it is the helpful move for a throwaway dev cert, and failing the
// start would leave the user with no obvious way forward.
func loadUsableDevCert(certPath string) (fingerprint string, ok bool) {
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		return "", false
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", false
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", false
	}
	if parsed.PublicKeyAlgorithm != x509.ECDSA {
		return "", false // RSA cannot be pinned by wtransport; regenerate.
	}
	now := time.Now()
	if now.Before(parsed.NotBefore) || now.Add(renewBefore).After(parsed.NotAfter) {
		return "", false
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), true
}

// devCertHosts assembles the subjectAltName list.
//
// The loopback literals are listed explicitly because localAddresses skips
// loopback interfaces — and a client dialling 127.0.0.1 still needs the name
// to match, which is the common case on one machine.
func devCertHosts(extra []string) ([]string, error) {
	names := map[string]bool{
		"localhost":       true,
		"127.0.0.1":       true,
		"::1":             true,
		emulatorHostAlias: true,
	}
	for _, h := range extra {
		if h = trimHostPort(h); h != "" {
			names[h] = true
		}
	}
	for _, h := range localAddresses() {
		names[h] = true
	}
	out := make([]string, 0, len(names))
	for h := range names {
		out = append(out, h)
	}
	// Sorted so the generated certificate is byte-stable for a given host
	// set, which keeps the fingerprint from churning between runs.
	sortStrings(out)
	return out, nil
}

// writeDevCert generates the key and self-signed certificate on disk.
func writeDevCert(certPath, keyPath string, hosts []string) (fingerprint string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate dev key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.Add(-devCertClockSkew),
		NotAfter:              now.Add(devCertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", fmt.Errorf("create dev cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal dev key: %w", err)
	}

	if err := writeFileAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", fmt.Errorf("write dev key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := writeFileAtomic(certPath, certPEM, 0o644); err != nil {
		return "", fmt.Errorf("write dev cert: %w", err)
	}

	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// writeFileAtomic writes via a temp file in the same directory so a crash
// mid-write cannot leave a half-written key where the daemon will try to use
// it. The private key is written first: a key without its cert is the safer
// half to have, since the next run regenerates rather than trusting a
// truncated file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// localAddresses lists non-loopback unicast IPs on up interfaces, so a phone
// on the same network can reach a dev server without the certificate needing
// hand-editing.
func localAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			out = append(out, ipnet.IP.String())
		}
	}
	return out
}

// trimHostPort reduces "host:port" or "[v6]:port" to a bare host. A bare IPv6
// literal is ambiguous with this, but the only IPv6 that can appear here is
// one the caller passed deliberately, and bracket form is unambiguous.
func trimHostPort(hostPort string) string {
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}
	return hostPort
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}