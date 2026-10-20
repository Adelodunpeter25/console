// Package version carries the server's own version string.
//
// It lives in its own package because two unrelated consumers need it: the
// CLI's --version flag and the server identity that a pairing URL advertises.
// Putting it in either would make the other import something it does not
// otherwise need.
package version

// Version is the Console server version. Bump on every release.
const Version = "1.0.1"
