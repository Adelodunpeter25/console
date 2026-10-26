package opentunnel

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	errNoPEM    = errors.New("opentunnel: no PEM data")
	errNotECDSA = errors.New("opentunnel: the private key is not ECDSA")
)

// The on-disk layout follows the OpenTunnel protocol's local storage section,
// so the identity is the same shape the official clients keep.
const (
	fileTunnel  = "tunnel.json"
	filePending = "pending.json"
	fileToken   = "token"
	fileKey     = "private-key.pem"
	fileCert    = "certificate.pem"
	fileChain   = "chain.pem"
)

type tunnelFile struct {
	ID                string     `json:"id"`
	Hostname          string     `json:"hostname"`
	CertificateExpiry *time.Time `json:"certificateExpiry,omitempty"`
}

type pendingFile struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	CSR      string `json:"csr"`
}

// Identity is a tunnel this machine owns. Until its certificate is ready it
// has a hostname and a pending request but cannot serve traffic.
type Identity struct {
	ID       string
	Hostname string
	Token    string
	Key      *ecdsa.PrivateKey

	// CSR is set while the certificate is still being provisioned.
	CSR string

	// Certificate is set once the certificate is ready.
	Certificate *tls.Certificate
	Expiry      time.Time
	certPEM     string
}

// Ready reports whether the tunnel has a certificate it can terminate TLS with.
func (i *Identity) Ready() bool { return i != nil && i.Certificate != nil }

// URL is the tunnel's public address.
func (i *Identity) URL() string { return "https://" + i.Hostname }

// Info is what the CLI needs to show an operator, read without touching the
// key or token.
type Info struct {
	Hostname string
	URL      string
	// Ready is false while the certificate is still being issued.
	Ready bool
}

// Store keeps one tunnel identity in a directory (0700, files 0600).
type Store struct{ Dir string }

func (s Store) path(name string) string { return filepath.Join(s.Dir, name) }

// ReadInfo reports the tunnel the directory holds, if any.
func ReadInfo(dir string) (Info, bool) {
	data, err := os.ReadFile(filepath.Join(dir, fileTunnel))
	if err != nil {
		return Info{}, false
	}
	var file tunnelFile
	if json.Unmarshal(data, &file) != nil || file.Hostname == "" {
		return Info{}, false
	}
	return Info{Hostname: file.Hostname, URL: "https://" + file.Hostname, Ready: file.CertificateExpiry != nil}, true
}

// writeFile replaces a file atomically, so a crash cannot leave a half-written
// token or key behind.
func (s Store) writeFile(name string, data []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("opentunnel: create %s: %w", s.Dir, err)
	}
	tmp := s.path(name + ".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("opentunnel: write %s: %w", name, err)
	}
	if err := os.Rename(tmp, s.path(name)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("opentunnel: replace %s: %w", name, err)
	}
	return nil
}

func (s Store) writeJSON(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.writeFile(name, data)
}

// savePending records a freshly created tunnel before its certificate is
// requested, so an interrupted provision resumes the same tunnel.
func (s Store) savePending(id, hostname, token string, key *ecdsa.PrivateKey, csr string) error {
	keyPEM, err := encodeKey(key)
	if err != nil {
		return err
	}
	if err := s.writeFile(fileToken, []byte(token)); err != nil {
		return err
	}
	if err := s.writeFile(fileKey, keyPEM); err != nil {
		return err
	}
	if err := s.writeJSON(filePending, pendingFile{ID: id, Hostname: hostname, CSR: csr}); err != nil {
		return err
	}
	// Last, so a tunnel.json always has its token and key beside it.
	return s.writeJSON(fileTunnel, tunnelFile{ID: id, Hostname: hostname})
}

// saveCertificate stores the issued certificate and marks the tunnel ready.
func (s Store) saveCertificate(id, hostname, certPEM, chainPEM string, expiry time.Time) error {
	if err := s.writeFile(fileCert, []byte(certPEM)); err != nil {
		return err
	}
	if err := s.writeFile(fileChain, []byte(chainPEM)); err != nil {
		return err
	}
	if err := s.writeJSON(fileTunnel, tunnelFile{ID: id, Hostname: hostname, CertificateExpiry: &expiry}); err != nil {
		return err
	}
	if err := os.Remove(s.path(filePending)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Clear forgets the tunnel. Only used when the server no longer knows it.
func (s Store) Clear() error {
	for _, name := range []string{fileTunnel, filePending, fileToken, fileKey, fileCert, fileChain} {
		if err := os.Remove(s.path(name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Load reads the stored identity, or nil when there is none.
func (s Store) Load() (*Identity, error) {
	data, err := os.ReadFile(s.path(fileTunnel))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opentunnel: read %s: %w", fileTunnel, err)
	}
	var file tunnelFile
	if err := json.Unmarshal(data, &file); err != nil || file.ID == "" || file.Hostname == "" {
		return nil, fmt.Errorf("opentunnel: %s is corrupt", fileTunnel)
	}

	token, err := os.ReadFile(s.path(fileToken))
	if err != nil {
		return nil, fmt.Errorf("opentunnel: read token: %w", err)
	}
	keyPEM, err := os.ReadFile(s.path(fileKey))
	if err != nil {
		return nil, fmt.Errorf("opentunnel: read private key: %w", err)
	}
	key, err := decodeKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("opentunnel: private key: %w", err)
	}
	identity := &Identity{ID: file.ID, Hostname: file.Hostname, Token: strings.TrimSpace(string(token)), Key: key}

	if certPEM, err := os.ReadFile(s.path(fileCert)); err == nil {
		chainPEM, _ := os.ReadFile(s.path(fileChain))
		if err := identity.setCertificate(string(certPEM), string(chainPEM), keyPEM); err != nil {
			return nil, err
		}
		return identity, nil
	}

	var pending pendingFile
	if data, err := os.ReadFile(s.path(filePending)); err == nil && json.Unmarshal(data, &pending) == nil {
		identity.CSR = pending.CSR
	}
	return identity, nil
}

// setCertificate installs a certificate and its chain.
func (i *Identity) setCertificate(certPEM, chainPEM string, keyPEM []byte) error {
	if keyPEM == nil {
		var err error
		if keyPEM, err = encodeKey(i.Key); err != nil {
			return err
		}
	}
	pair, err := tls.X509KeyPair([]byte(strings.TrimSpace(certPEM)+"\n"+strings.TrimSpace(chainPEM)+"\n"), keyPEM)
	if err != nil {
		return fmt.Errorf("opentunnel: the certificate does not match the key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("opentunnel: read the certificate: %w", err)
	}
	pair.Leaf = leaf
	i.Certificate, i.Expiry, i.certPEM, i.CSR = &pair, leaf.NotAfter, certPEM, ""
	return nil
}
