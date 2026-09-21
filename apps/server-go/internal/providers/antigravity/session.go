// Antigravity per-session request envelope. Port of
// apps/server/providers/src/antigravity/session-envelope.ts.
package antigravity

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// SessionState is stable per Agent instance (per Console session).
type SessionState struct {
	mu              sync.Mutex
	AgentID         string
	TrajectoryID    string
	SessionID       string
	StepIndex       int
	LastExecutionID string
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewSessionState mirrors createSessionState.
func NewSessionState() *SessionState {
	agentID := newUUID()
	hex := strings.ReplaceAll(newUUID(), "-", "")
	sessionIDInt := new(big.Int)
	sessionIDInt.SetString(hex, 16)
	return &SessionState{
		AgentID: agentID, TrajectoryID: newUUID(),
		SessionID: sessionIDInt.String(), StepIndex: 0,
	}
}

// Envelope is the per-request session envelope.
type Envelope struct {
	Labels    map[string]string
	SessionID string
	RequestID string
}

var modelEnums = map[string]string{
	"gemini-3.5-flash-extra-low": "MODEL_PLACEHOLDER_M187",
	"gemini-3.5-flash-low":       "MODEL_PLACEHOLDER_M20",
	"gemini-3-flash-agent":       "MODEL_PLACEHOLDER_M132",
	"gemini-3.1-pro-low":         "MODEL_PLACEHOLDER_M36",
	"gemini-pro-agent":           "MODEL_PLACEHOLDER_M16",
}

// BuildEnvelope advances the session state one step and builds the
// requestId/labels/sessionId envelope for one request. Mirrors buildEnvelope.
func BuildEnvelope(state *SessionState, wireModelID string) Envelope {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.StepIndex++
	step := state.StepIndex
	requestID := fmt.Sprintf("agent/%s/%d/%s/%d", state.AgentID, time.Now().UnixMilli(), state.TrajectoryID, step)

	isClaude := strings.Contains(strings.ToLower(wireModelID), "claude")

	labels := map[string]string{
		"trajectory_id":            state.TrajectoryID,
		"last_step_index":          fmt.Sprintf("%d", step-1),
		"used_claude":              fmt.Sprintf("%t", isClaude),
		"used_claude_conservative": fmt.Sprintf("%t", isClaude),
	}
	if modelEnum, ok := modelEnums[wireModelID]; ok {
		labels["model_enum"] = modelEnum
	}
	if state.LastExecutionID != "" {
		labels["last_execution_id"] = state.LastExecutionID
	}

	return Envelope{Labels: labels, SessionID: state.SessionID, RequestID: requestID}
}

// UpdateLastExecutionID records the response id for the next request.
func UpdateLastExecutionID(state *SessionState, responseID string) {
	if responseID == "" {
		return
	}
	state.mu.Lock()
	state.LastExecutionID = responseID
	state.mu.Unlock()
}
