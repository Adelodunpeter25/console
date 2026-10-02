package mcp

// OpenBrowser is a no-op on the server. In our client/server architecture,
// the desktop frontend is responsible for automatically opening the OAuth
// URL in the user's browser whenever a server transitions to needs_auth.
func OpenBrowser(u string) error {
	return nil
}
