package loop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func newMessageID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return "msg_" + hex.EncodeToString(b)
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
