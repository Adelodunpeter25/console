// Per-chat composer drafts against the global DB.
package services

import (
	"database/sql"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

type DraftService struct {
	manager *db.DB
}

func NewDraftService(manager *db.DB) *DraftService {
	return &DraftService{manager: manager}
}

// Get returns the stored draft, or nil when the chat has none.
func (s *DraftService) Get(sessionID string) (*consolev1.ChatDraft, error) {
	var body string
	err := s.manager.Global().QueryRow(
		`SELECT body FROM chat_drafts WHERE session_id = ?`, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	draft := &consolev1.ChatDraft{}
	if err := protojson.Unmarshal([]byte(body), draft); err != nil {
		return nil, err
	}
	return draft, nil
}

// Put stores the draft when it is not older than the stored one. It returns
// whether it was applied and the draft the caller should hold afterwards.
// An empty draft deletes the row so cleared drafts do not accumulate.
func (s *DraftService) Put(draft *consolev1.ChatDraft) (bool, *consolev1.ChatDraft, error) {
	if draft.SessionId == "" {
		return false, nil, fmt.Errorf("sessionId is required.")
	}
	tx, err := s.manager.Global().Begin()
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()

	var storedAt int64
	var storedBody string
	err = tx.QueryRow(`SELECT updated_at, body FROM chat_drafts WHERE session_id = ?`,
		draft.SessionId).Scan(&storedAt, &storedBody)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, nil, err
	}
	if err == nil && storedAt > draft.UpdatedAt {
		stored := &consolev1.ChatDraft{}
		if err := protojson.Unmarshal([]byte(storedBody), stored); err != nil {
			return false, nil, err
		}
		return false, stored, nil
	}

	if draft.Text == "" && len(draft.Comments) == 0 {
		// Keep a tombstone-free table, but still report the cleared draft.
		if _, err := tx.Exec(`DELETE FROM chat_drafts WHERE session_id = ?`, draft.SessionId); err != nil {
			return false, nil, err
		}
		return true, draft, tx.Commit()
	}
	body, err := protojson.Marshal(draft)
	if err != nil {
		return false, nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO chat_drafts (session_id, body, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		draft.SessionId, string(body), draft.UpdatedAt); err != nil {
		return false, nil, err
	}
	return true, draft, tx.Commit()
}
