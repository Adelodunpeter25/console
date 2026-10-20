// Pairing output for the CLI.
//
// The operator needs one thing they can copy and one thing they can scan.
// The pairing code is short enough to read out loud and type, and the URL
// carries the same code so a camera can pick it up instead.
//
// The grant is minted directly against the database rather than asked of
// the running server, so `console status` works before the daemon is up,
// and so no unauthenticated HTTP endpoint has to exist purely to hand out
// credentials.
package commands

import (
	"fmt"
	"net"
	"os"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

// PairingInfo is everything the operator needs to connect a device.
type PairingInfo struct {
	// Host is the server's address as "host:port" — the form the client's
	// host field takes.
	Host string
	// Code is the short pairing code.
	Code string
	// URL is the whole thing, with the code in the fragment.
	URL string
	// ServerName is the server's display name, for the confirmation prompt
	// on the client.
	ServerName string
	// ServerID identifies the server the code came from.
	ServerID string
}

// Pairing mints a one-time pairing code and describes how to use it.
func Pairing(dir string) (PairingInfo, error) {
	database, err := db.Open(db.OpenOptions{StorageDir: dir})
	if err != nil {
		return PairingInfo{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	identity, err := auth.LoadServerIdentityIn(dir)
	if err != nil {
		return PairingInfo{}, err
	}

	// A server that has never been paired gets the first-run grant; one
	// that has gets an operator grant. Both are one-time and unscoped, so
	// either pairs one device — the distinction is in the audit trail, not
	// in what the code can do.
	code := ""
	if paired, err := database.CountAllAuthDevices(); err == nil && paired == 0 {
		code, _, err = auth.CreateGrant(database, auth.PairingKind, "", "", "", auth.PairingTTL())
		if err != nil {
			return PairingInfo{}, fmt.Errorf("create pairing code: %w", err)
		}
	}
	if code == "" {
		code, _, err = auth.CreateGrant(database, auth.OperatorKind, "", "", "", auth.PairingTTL())
		if err != nil {
			return PairingInfo{}, fmt.Errorf("create pairing code: %w", err)
		}
	}

	base := publicBaseURL(dir)
	return PairingInfo{
		Host:       hostPort(base),
		Code:       code,
		URL:        auth.PairingURL(base, identity.ID, code),
		ServerName: identity.Name,
		ServerID:   identity.ID,
	}, nil
}

// PrintPairing writes the pairing details to stdout.
//
// The code is the thing a user types; the URL is the thing a user copies;
// the QR is the thing a user scans. All three carry the same one-time
// code, so only one of them is ever needed.
//
// The URL comes before the QR on purpose: a QR is a picture, and a user
// who wants to copy something needs a line of text above it.
func PrintPairing(dir string) {
	info, err := Pairing(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create a pairing code: %v\n", err)
		return
	}
	fmt.Printf("Pair this device by pasting the code into the Console app:\n\n")
	fmt.Printf("  Host: %s\n", info.Host)
	fmt.Printf("  Code: %s\n\n", info.Code)
	fmt.Printf("Or paste the whole URL:\n  %s\n\n", info.URL)
	fmt.Println("Or scan this:")
	fmt.Println()
	printQR(info.URL)
}

// printQR draws a QR code in the terminal.
//
// A camera pointed at the terminal picks the code up without anyone
// transcribing it — which is the point of having a code short enough to
// read but a URL worth scanning.
//
// The compact rendering is used: it is half as wide, so the code does not
// wrap in a narrow terminal (a wrapped QR does not scan), and it uses the
// half-block characters every modern terminal renders.
func printQR(content string) {
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		// A QR that cannot be drawn is not worth failing over: the code
		// and the URL above are still on screen.
		return
	}
	fmt.Println(code.ToSmallString(false))
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

// hostPort renders a base URL as "host:port" — the form a client's host
// field takes, and the shorter of the two to read.
func hostPort(baseURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
	return strings.TrimRight(trimmed, "/")
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
