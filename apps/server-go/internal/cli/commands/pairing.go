// Pairing URL generation for the CLI.
//
// The operator needs one string they can copy and paste into a client. It
// has to name the server (so the client can confirm it is talking to the
// right machine) and carry a one-time grant (so the paste alone is
// enough to pair).
//
// The grant is minted directly against the database rather than asked of
// the running server, so `console status` works even before the daemon is
// up, and so no unauthenticated HTTP endpoint has to exist purely to hand
// out credentials.
package commands

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

// PairingURL mints a one-time grant and renders the copy-pasteable URL.
//
// Every call mints a fresh grant: running the CLI is an explicit act by
// someone who already has the server's signing secret on disk, so handing
// them a pairing invitation grants them nothing they did not already have.
// The grant is single-use and short-lived, so even a leaked URL buys one
// pairing and nothing more.
//
// dir is the storage directory of the server the grant is for. It matters:
// a dev server and a production server have separate databases, and a
// grant written to the wrong one would never be seen by the server the
// client is about to talk to.
func PairingURL(dir string) (string, error) {
	database, err := db.Open(db.OpenOptions{StorageDir: dir})
	if err != nil {
		return "", fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	identity, err := auth.LoadServerIdentityIn(dir)
	if err != nil {
		return "", err
	}

	// A server that has never been paired gets the first-run grant; one
	// that has gets an operator grant. Both are one-time and unscoped, so
	// either pairs one device — the distinction is in the audit trail, not
	// in what the grant can do.
	grant := ""
	if paired, err := database.CountAllAuthDevices(); err == nil && paired == 0 {
		grant, _, err = auth.CreateGrant(database, auth.PairingKind, "", "", "", auth.PairingTTL())
		if err != nil {
			return "", fmt.Errorf("create pairing grant: %w", err)
		}
	}
	if grant == "" {
		grant, _, err = auth.CreateGrant(database, auth.OperatorKind, "", "", "", auth.PairingTTL())
		if err != nil {
			return "", fmt.Errorf("create pairing grant: %w", err)
		}
	}
	return auth.PairingURL(publicBaseURL(dir), identity.ID, grant), nil
}

// publicBaseURL is the address a client should use to reach this server.
//
// The environment wins over the saved config, because that is the order the
// server itself resolves its listen address in: a server started with
// PORT=3999 listens on 3999, whatever the saved config says, and a URL
// pointing somewhere else would simply not connect.
//
// The configured host is often 0.0.0.0 — a bind address, not a dialable
// one. A wildcard is replaced with the machine's LAN address so the URL
// works from a phone, and with loopback when there is no LAN at all.
func publicBaseURL(dir string) string {
	cfg := daemon.LoadConfig(dir)
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = strings.TrimSpace(cfg.Host)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = lanAddress()
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strings.TrimSpace(cfg.Port)
	}
	if port == "" {
		port = "3000"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

// lanAddress finds the machine's address on the local network.
//
// A client on the same LAN (a phone) can only reach the server by a
// routable address, so 0.0.0.0 has to become something real. Loopback is
// the fallback when the machine has no external interface.
func lanAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ip := ipNet.IP.To4(); ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

// PrintPairingURL writes the pairing URL to stdout, or an explanation when
// there is nothing to print.
//
// The URL is the one line a user copies, so it is printed on its own line
// with no decoration around it.
func PrintPairingURL(dir string) {
	url, err := PairingURL(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create a pairing URL: %v\n", err)
		return
	}
	fmt.Println("Pair this device by pasting the URL below into the Console app:")
	fmt.Println(url)
}
