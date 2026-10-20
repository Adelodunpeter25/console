// Device credential helpers shared by the Fiber routes.
//
// The token *minting* endpoint that used to live here is gone: a device
// credential is issued only by exchanging a one-time pairing grant
// (see RegisterPairingRoutes). A route that minted a token from a
// caller-supplied device id was proof of nothing — anyone who guessed an id
// got a credential.
//
// The query-string form (`?token=`) is gone too, for ordinary HTTP. URLs
// leak through access logs, proxy logs, browser history and Referer headers,
// so a credential in one is a credential in a log file. The WebTransport
// handshake keeps its own scoped copy of that path, because a CONNECT
// request from some clients cannot carry headers.
package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// DeviceIDKey is the Fiber local the authenticated device id lands in.
const DeviceIDKey = "authenticatedDeviceID"

// bearerToken extracts a bearer credential. The scheme matches
// case-insensitively per RFC 9110; anything else (including a bare token
// with no scheme) is rejected so callers cannot accidentally send one form
// while the server reads another.
func bearerToken(header string) string {
	scheme, credentials, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(strings.TrimSpace(scheme), "bearer") {
		return ""
	}
	return strings.TrimSpace(credentials)
}

// DeviceIDFromContext reads the device id the middleware stored.
func DeviceIDFromContext(c *fiber.Ctx) (string, bool) {
	deviceID, ok := c.Locals(DeviceIDKey).(string)
	return deviceID, ok && deviceID != ""
}
