package commands

// `console cert-fingerprint` — print the SHA-256 a client needs in order to
// pin the development certificate.
//
// This exists so tooling does not have to reimplement cert handling in shell.
// It used to be `openssl x509 ... | cut -d= -f2 | tr -d :`, which meant every
// caller needed openssl, had to agree on the certificate's location, and
// broke silently the day that location changed.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

// CertFingerprint prints the pinning fingerprint for this mode's development
// certificate, generating it if it is missing or expired.
//
// Generating on read rather than only on `start --wt` is deliberate: the
// common workflow starts the desktop client and the server in either order,
// and a client that is pointed at a fingerprint must never be handed one for
// a certificate the server will later replace.
func CertFingerprint(dev bool) error {
	if dev {
		_ = os.Setenv("CONSOLE_ENV", "dev")
	} else if os.Getenv("CONSOLE_ENV") == "" {
		_ = os.Setenv("CONSOLE_ENV", "production")
	}
	if err := daemon.EnsureConsoleDir(); err != nil {
		return err
	}
	dir := daemon.ConsoleDir()
	cert, err := wt.EnsureDevCert(
		filepath.Join(dir, "wt-dev.crt"),
		filepath.Join(dir, "wt-dev.key"),
		nil,
	)
	if err != nil {
		return err
	}
	fmt.Println(cert.Fingerprint)
	return nil
}