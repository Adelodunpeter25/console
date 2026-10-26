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
	"path/filepath"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
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
	// Remote is the server's permanent public address through the tunnel, or
	// empty when remote access is off or has not been created yet.
	Remote string
	// RemoteReady is false while the tunnel's certificate is still being issued.
	RemoteReady bool
}

// tunnelDir is where the server keeps its tunnel identity.
func tunnelDir(dir string) string { return filepath.Join(dir, "tunnel") }

// remoteInfo reads the tunnel the server created for itself.
func remoteInfo(dir string) (opentunnel.Info, bool) {
	return opentunnel.ReadInfo(tunnelDir(dir))
}

// WaitForRemote gives a server that is still starting a moment to create its
// tunnel: the hostname exists as soon as the service answers, well before the
// certificate is issued.
func WaitForRemote(dir string, timeout time.Duration) {
	if remoteOff() {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := remoteInfo(dir); ok {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func remoteOff() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_REMOTE"))) {
	case "0", "off", "false", "no", "disabled":
		return true
	}
	return false
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
	info := PairingInfo{
		Host:       hostPort(base),
		Code:       code,
		ServerName: identity.Name,
		ServerID:   identity.ID,
	}
	// A phone off the local network can only use the tunnel, so the pairing
	// URL (and its QR) carry the remote address whenever there is one.
	if remote, ok := remoteInfo(dir); ok && strings.TrimSpace(os.Getenv("CONSOLE_PUBLIC_URL")) == "" {
		info.Remote, info.RemoteReady = remote.URL, remote.Ready
		base = remote.URL
	}
	info.URL = auth.PairingURL(base, identity.ID, code)
	return info, nil
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
	if info.Remote != "" {
		note := ""
		if !info.RemoteReady {
			note = "  (getting its certificate; usable in a minute)"
		}
		fmt.Printf("  Remote url : %s%s\n", info.Remote, note)
	}
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
// CONSOLE_PUBLIC_URL wins outright: a server behind a tunnel, reverse proxy
// or load balancer is not reachable at the address it binds, so the operator
// states the address clients should use instead. It carries its own scheme,
// so an https endpoint stays https.
//
// The environment wins over the saved config, because that is the order the
// server itself resolves its listen address in: a server started with
// PORT=3999 listens on 3999, whatever the saved config says, and a URL
// pointing somewhere else would simply not connect.
//
// The configured host is often 0.0.0.0 — a bind address, not a dialable
// one. A wildcard is replaced with the machine's dialable address so the URL
// works from a phone, and with loopback when there is no LAN at all.
func publicBaseURL(dir string) string {
	if override := strings.TrimSpace(os.Getenv("CONSOLE_PUBLIC_URL")); override != "" {
		return strings.TrimRight(override, "/")
	}
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
//
// The scheme is dropped only when it is the default one. A client's host
// field assumes http when it carries no scheme, so an https address has to
// keep its own or it would be spoken to in the wrong protocol.
func hostPort(baseURL string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	return strings.TrimPrefix(trimmed, "http://")
}

// lanAddress finds the address a client should use to reach this machine.
//
// The default route's source address wins: it is the address the kernel
// itself picks for outbound traffic, so it is the one the outside world can
// reach. A cloud host usually carries several routable addresses at once —
// docker0, a VPC NIC, a public NIC — and the first one the interface list
// happens to yield is frequently the wrong one.
//
// Loopback is the fallback when the machine has no external interface.
func lanAddress() string {
	return SelectDialableAddress(defaultRouteIP(), interfaceAddresses())
}

// defaultRouteIP asks the kernel which source address it would use for an
// outbound connection.
//
// A UDP "connect" is a local route lookup — it sends no packets — so this
// costs no network traffic and needs no external service. It fails only when
// the machine has no default route at all, such as an isolated container.
func defaultRouteIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return nil
	}
	ip := addr.IP
	if ip.IsLoopback() || ip.IsUnspecified() {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return nil
}

// interfaceAddresses lists the routable IPv4 addresses on the machine's
// interfaces, in the order the OS reports them.
func interfaceAddresses() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
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
				out = append(out, ip)
			}
		}
	}
	return out
}

// SelectDialableAddress picks the address a client should use to reach a
// machine.
//
// routeIP is the source address the kernel would use for outbound traffic,
// and is nil when the machine has no default route. interfaceIPs are the
// routable addresses found on the machine's interfaces.
//
// The default route's address is preferred because it is the one the outside
// world can reach; the interface addresses are only a fallback for a machine
// with no default route, where the first routable address is the best guess
// available.
func SelectDialableAddress(routeIP net.IP, interfaceIPs []net.IP) string {
	if routeIP != nil {
		if v4 := routeIP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsUnspecified() {
			return v4.String()
		}
	}
	for _, ip := range interfaceIPs {
		if v4 := ip.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsUnspecified() {
			return v4.String()
		}
	}
	return "127.0.0.1"
}
