// The pairing URL: the one string a user copies and pastes to connect a
// client to this server.
//
// Shape:
//
//	https://server.example/console/pair#v=1&server=<server-id>&grant=<one-time-grant>
//
// Two decisions are load-bearing:
//
//  1. The secret (the grant) lives in the **fragment**. A fragment is never
//     sent to the server by the browser, so the grant does not appear in
//     access logs, proxy logs, or Referer headers. A URL that carried it in
//     the query string would leak it to every hop.
//
//  2. The base URL is the server's own address, so the URL alone is enough
//     to connect: a client that receives it needs no separate "which
//     server" step. That is what makes copy-paste work.
//
// A client parses the fragment, checks that the server id matches the one
// it is talking to, and exchanges the grant for its own device token.
package auth

import (
	"fmt"
	"net/url"
	"strings"
)

// PairingURLPath is the path a pairing URL points at. It is never
// requested over the network (the secret is in the fragment) — it exists
// so the URL is recognisable and so a browser hitting it gets a sensible
// answer instead of a 404.
const PairingURLPath = "/console/pair"

// pairingURLVersion is the format version in the fragment. A client that
// does not recognise it refuses the URL rather than guessing.
const pairingURLVersion = "1"

// PairingURL renders the copy-pasteable pairing URL for a grant.
//
// baseURL is the server's externally reachable address (scheme, host and
// port). The fragment carries the version, the server id and the grant.
func PairingURL(baseURL, serverID, grant string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "http://localhost:3000"
	}
	fragment := url.Values{}
	fragment.Set("v", pairingURLVersion)
	fragment.Set("server", serverID)
	fragment.Set("grant", grant)
	return trimmed + PairingURLPath + "#" + fragment.Encode()
}

// ParsedPairingURL is the content of a pairing URL.
type ParsedPairingURL struct {
	// BaseURL is the server's address: scheme, host and port.
	BaseURL string
	// ServerID is the server the URL was minted by.
	ServerID string
	// Grant is the one-time pairing grant.
	Grant string
}

// ParsePairingURL reads a pairing URL back into its parts.
//
// It is strict on purpose. A URL that cannot be parsed exactly must be
// rejected, not repaired: the grant is a credential, and a lenient parser
// is how a malformed input becomes a wrong-server pairing.
func ParsePairingURL(raw string) (ParsedPairingURL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ParsedPairingURL{}, fmt.Errorf("pairing URL is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ParsedPairingURL{}, fmt.Errorf("parse pairing URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ParsedPairingURL{}, fmt.Errorf("pairing URL must be http or https")
	}
	if parsed.Host == "" {
		return ParsedPairingURL{}, fmt.Errorf("pairing URL has no host")
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		return ParsedPairingURL{}, fmt.Errorf("parse pairing URL fragment: %w", err)
	}
	if fragment.Get("v") != pairingURLVersion {
		return ParsedPairingURL{}, fmt.Errorf("unsupported pairing URL version %q", fragment.Get("v"))
	}
	serverID := strings.TrimSpace(fragment.Get("server"))
	grant := strings.TrimSpace(fragment.Get("grant"))
	if serverID == "" {
		return ParsedPairingURL{}, fmt.Errorf("pairing URL has no server id")
	}
	if grant == "" {
		return ParsedPairingURL{}, fmt.Errorf("pairing URL has no grant")
	}
	return ParsedPairingURL{
		BaseURL:  parsed.Scheme + "://" + parsed.Host,
		ServerID: serverID,
		Grant:    grant,
	}, nil
}
