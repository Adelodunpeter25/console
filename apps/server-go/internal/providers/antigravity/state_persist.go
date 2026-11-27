package antigravity

import (
	"github.com/MoeForge/moego/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// Conversation identity (session id, trajectory id, step counter, last
// execution id) is what lets the endpoint recognise a continuing conversation
// and reuse its cache. It is saved to disk so a server restart resumes the
// same identity instead of starting a new, cold one.
const (
	stateFileName    = "antigravity-sessions.json"
	stateMaxAge      = 30 * 24 * time.Hour
	stateMaxSessions = 500
)

type persistedState struct {
	AgentID         string    `json:"agentId"`
	TrajectoryID    string    `json:"trajectoryId"`
	SessionID       string    `json:"sessionId"`
	StepIndex       int       `json:"stepIndex"`
	LastExecutionID string    `json:"lastExecutionId,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func stateFilePath() string {
	return filepath.Join(utils.ConsoleStorageDir(), stateFileName)
}

// loadPersistedStates reads the saved identities, dropping stale ones.
func loadPersistedStates() map[string]persistedState {
	out := map[string]persistedState{}
	raw, err := os.ReadFile(stateFilePath())
	if err != nil {
		return out
	}
	var saved map[string]persistedState
	if json.Unmarshal(raw, &saved) != nil {
		return out
	}
	cutoff := time.Now().Add(-stateMaxAge)
	for id, s := range saved {
		if s.UpdatedAt.After(cutoff) && s.SessionID != "" && s.AgentID != "" && s.TrajectoryID != "" {
			out[id] = s
		}
	}
	return out
}

// snapshot copies the state for saving.
func (s *SessionState) snapshot() persistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return persistedState{
		AgentID: s.AgentID, TrajectoryID: s.TrajectoryID, SessionID: s.SessionID,
		StepIndex: s.StepIndex, LastExecutionID: s.LastExecutionID, UpdatedAt: time.Now(),
	}
}

func (p persistedState) restore() *SessionState {
	return &SessionState{
		AgentID: p.AgentID, TrajectoryID: p.TrajectoryID, SessionID: p.SessionID,
		StepIndex: p.StepIndex, LastExecutionID: p.LastExecutionID,
	}
}

// savePersistedState writes one conversation's identity, keeping the file
// bounded. Failures are logged and never affect the request.
func savePersistedState(conversationID string, state *SessionState) {
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()
	saved := loadPersistedStates()
	saved[conversationID] = state.snapshot()
	for len(saved) > stateMaxSessions {
		oldestID, oldest := "", time.Now()
		for id, s := range saved {
			if s.UpdatedAt.Before(oldest) {
				oldestID, oldest = id, s.UpdatedAt
			}
		}
		delete(saved, oldestID)
	}
	raw, err := json.Marshal(saved)
	if err != nil {
		return
	}
	path := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.Warn("antigravity state save failed", "error", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		slog.Warn("antigravity state save failed", "error", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		slog.Warn("antigravity state save failed", "error", err)
	}
}

// SaveState persists one conversation's identity (exported for tests).
func SaveState(conversationID string, state *SessionState) { savePersistedState(conversationID, state) }

// ResetStateStore forgets in-memory identities (tests simulate a restart).
func ResetStateStore() {
	stateStoreMu.Lock()
	stateStore = map[string]*SessionState{}
	stateStoreMu.Unlock()
}
