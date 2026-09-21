package loop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func newMessageID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return "msg_" + hex.EncodeToString(b)
}

// newTurnID mints an opaque turn id (TS parity: randomUUID per turn).
func newTurnID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// jsonAny converts raw JSON into generic map/any for event payloads.
func jsonAny(raw json.RawMessage) any {
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

// asToolError is errors.As for *tools.ToolError.
func asToolError(err error, target **tools.ToolError) bool {
	if te, ok := err.(*tools.ToolError); ok {
		*target = te
		return true
	}
	return false
}
