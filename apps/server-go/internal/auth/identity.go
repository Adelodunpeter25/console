// Server identity: the stable, public name of this server.
//
// A device that is about to hand over a credential needs to know *which*
// server it is talking to, and a human reading a pairing URL needs a label
// that is not a UUID. Both come from here.
//
// The id is a random UUID minted once and persisted beside the signing
// secret. It is not a secret — it appears in pairing URLs and in the
// QR payload — but it is also not guessable, because a device checks that
// the id in the URL matches the server it is about to pair with. A
// mismatched id means the user pasted a URL from a different machine.
//
// The file is written 0600 in a 0700 directory, like the signing secret:
// the id is public, but the file also records the display name, and
// keeping one convention means one set of permission assertions.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/version"
)

// identityFileVersion guards the on-disk shape. A future format change
// bumps it; an unreadable or future-versioned file is replaced rather
// than silently misread.
const identityFileVersion = 1

// ServerIdentity is this server's public identity.
type ServerIdentity struct {
	// ID is a random, stable identifier for this server.
	ID string
	// Name is the human-readable label shown in pairing UIs.
	Name string
	// Version is the server's own version string.
	Version string
	// BaseURL is the address the client should talk to, when the server
	// knows it. Empty means "the server did not claim one" — the client
	// then uses the address it was given.
	BaseURL string
}

// identityFile is the on-disk shape.
type identityFile struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// ServerIdentityPath resolves the identity file: CONSOLE_SERVER_STATE_PATH
// wins outright (tests, unusual deployments), else <storage>/server-identity.json.
func ServerIdentityPath() string {
	if override := os.Getenv("CONSOLE_SERVER_STATE_PATH"); override != "" {
		return override
	}
	return filepath.Join(utils.ConsoleStorageDir(), "server-identity.json")
}

// ServerIdentityPathIn resolves the identity file inside a specific
// storage directory.
//
// The CLI needs this: it mints a pairing grant for a *running* server,
// which may be the dev one while the CLI itself is pointed at production.
func ServerIdentityPathIn(dir string) string {
	if override := os.Getenv("CONSOLE_SERVER_STATE_PATH"); override != "" {
		return override
	}
	return filepath.Join(dir, "server-identity.json")
}

// LoadServerIdentity reads the persisted identity, creating it on first run.
func LoadServerIdentity() (ServerIdentity, error) {
	return LoadServerIdentityIn(utils.ConsoleStorageDir())
}

// LoadServerIdentityIn reads the persisted identity in a specific storage
// directory, creating it on first run.
//
// The display name is resolved at load time rather than frozen at creation:
// an explicit CONSOLE_SERVER_NAME wins, then the name recorded in the file,
// then the machine hostname. A file created before the machine was renamed
// therefore follows the rename unless the operator pinned a name.
//
// The signing secret and the identity live side by side, so a failure to
// read one is a failure to read the other: this returns an error rather
// than minting a throwaway identity, because a client that trusts a
// server id must be able to trust that it is stable.
func LoadServerIdentityIn(dir string) (ServerIdentity, error) {
	path := ServerIdentityPathIn(dir)
	file, err := readIdentityFile(path)
	if err != nil {
		return ServerIdentity{}, err
	}
	if file == nil {
		file, err = createIdentityFile(path)
		if err != nil {
			return ServerIdentity{}, err
		}
	}
	name := resolveServerName(file.Name)
	if name != file.Name {
		// An explicit name outranks the stored one, so persist it — otherwise
		// the file and the reported identity would disagree.
		file.Name = name
		if err := writeIdentityFile(path, file); err != nil {
			return ServerIdentity{}, err
		}
	}
	return ServerIdentity{ID: file.ID, Name: name, Version: version.Version}, nil
}

// resolveServerName picks the display name: explicit configuration first,
// then the stored name, then the hostname.
func resolveServerName(stored string) string {
	if explicit := strings.TrimSpace(os.Getenv("CONSOLE_SERVER_NAME")); explicit != "" {
		return explicit
	}
	if strings.TrimSpace(stored) != "" {
		return stored
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return "Console Server"
}

// readIdentityFile reads and validates the file. A missing file returns
// (nil, nil) — the caller creates it. A corrupt file is an error: silently
// replacing it would mint a new id, and every device already paired would
// be talking to a server that claims to be a different one.
func readIdentityFile(path string) (*identityFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read server identity: %w", err)
	}
	var file identityFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse server identity: %w", err)
	}
	if file.Version != identityFileVersion {
		return nil, fmt.Errorf("server identity file version %d is not supported", file.Version)
	}
	if strings.TrimSpace(file.ID) == "" {
		return nil, fmt.Errorf("server identity file has no id")
	}
	return &file, nil
}

// createIdentityFile mints and persists a new identity.
//
// The id is eight hex characters rather than a full UUID: it appears in
// every pairing URL, where a 36-character id makes the URL twice as long
// as it needs to be, and it identifies a server among the handful an
// operator actually has. It is not a secret — it is in the URL on purpose
// — it is only ever checked for equality.
func createIdentityFile(path string) (*identityFile, error) {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate server id: %w", err)
	}
	file := &identityFile{
		Version:   identityFileVersion,
		ID:        hex.EncodeToString(raw),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := writeIdentityFile(path, file); err != nil {
		return nil, err
	}
	return file, nil
}

// writeIdentityFile persists the identity with restrictive permissions.
func writeIdentityFile(path string, file *identityFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create server identity directory: %w", err)
	}
	// MkdirAll leaves an existing directory's mode alone, and anything
	// looser than 0700 here would weaken the files below it.
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure server identity directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode server identity: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write server identity: %w", err)
	}
	return nil
}
