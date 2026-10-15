package commands

// WebTransport bring-up for `console start --wt`.
//
// The goal is that `--wt` is the *only* thing anyone has to type: no openssl,
// no certificate files to locate, no fingerprint to compute. The certificate is
// generated on demand and its fingerprint printed, ready to paste into a
// client that pins it.

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

// defaultWTTPPort is the QUIC port when none is given. It matches the long-
// standing `make dev-server-wt` default so existing muscle memory keeps
// working.
const defaultWTTPPort = "14443"

// setupWebTransport generates (or reuses) the development certificate the QUIC
// listener needs and returns it.
//
// The certificate lives inside the console directory rather than a fixed path
// so `console start --dev --wt` and `console start --wt` get separate
// certificates — the desktop already keeps separate dev and production
// storage, and a shared cert would let one trust the other's daemon.
func setupWebTransport(options StartOptions, host string) (wt.DevCert, error) {
	dir := daemon.ConsoleDir()
	return wt.EnsureDevCert(
		filepath.Join(dir, "wt-dev.crt"),
		filepath.Join(dir, "wt-dev.key"),
		wtBindHosts(host),
	)
}

// wtAddr builds the QUIC listen address. The QUIC listener follows the HTTP
// listener's host: `console start` already binds 0.0.0.0 by default, so
// inheriting it means a phone on the same network reaches QUIC exactly when it
// can reach HTTP, with no extra flag to remember.
func wtAddr(host, port string) string {
	if port == "" {
		port = defaultWTTPPort
	}
	// A wildcard or already-qualified host is passed through, so we do not
	// double up brackets for IPv6.
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

// wtBindHosts lists the names a client may use to reach this server, so they
// land in the certificate's SAN list. The wildcard bind address itself is not
// a name a client dials, so it contributes nothing.
func wtBindHosts(host string) []string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return nil
	}
	return []string{strings.Trim(host, "[]")}
}

// printWebTransport reports the QUIC endpoint and, importantly, the certificate
// fingerprint — the one value a client needs that it cannot derive on its own.
func printWebTransport(cert wt.DevCert, addr string) {
	if cert.Generated {
		fmt.Println("WebTransport: enabled (generated a development certificate)")
	} else {
		fmt.Println("WebTransport: enabled")
	}
	fmt.Printf("QUIC:         %s\n", addr)
	fmt.Printf("Certificate:  %s\n", cert.CertPath)
	fmt.Printf("Fingerprint:  %s\n", cert.Fingerprint)
	fmt.Println("Pin it in a client with: CONSOLE_WT_TLS=pin:" + cert.Fingerprint)
	fmt.Printf("Health:       %s\n", wtHealthURL(addr))
}

// wtHealthURL renders the https origin a client should target for this QUIC
// listener. Wildcard binds are not dialable names, so localhost is the useful
// stand-in for a local client.
func wtHealthURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "https://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	// JoinHostPort brackets IPv6 literals for us.
	return "https://" + net.JoinHostPort(host, port)
}