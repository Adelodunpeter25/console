// Per-session/device request identity for Anthropic's OAuth (Claude
// Pro/Max subscription) endpoint. Console's Claude requests previously sent
// none of this, which left every request from a machine indistinguishable
// from every other to Anthropic — this mirrors what Claude Code's own CLI
// (and oh-my-pi's Anthropic provider) send on every request.
package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// deviceIDDomain namespaces the device-id hash so it can't collide with
// identifiers derived from the same install id for unrelated purposes.
const deviceIDDomain = "console-claude-device-id-v1:"

// DeviceID derives a stable per-installation device id from the machine's
// persistent install id.
func DeviceID() string {
	sum := sha256.Sum256([]byte(deviceIDDomain + utils.InstallID()))
	return hex.EncodeToString(sum[:])
}

// metadataUserID builds the `metadata.user_id` JSON value Anthropic expects
// from OAuth clients: a stable device id plus the per-conversation session
// id. Returns "" when sessionID is empty (nothing to identify).
func metadataUserID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	payload, err := json.Marshal(map[string]string{
		"device_id":  DeviceID(),
		"session_id": sessionID,
	})
	if err != nil {
		return ""
	}
	return string(payload)
}
