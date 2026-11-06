// Message operations: append, replace.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func (s *Service) AppendMessage(sessionID string, msg types.AgentMessage) error {
	return s.AppendMessages(sessionID, []types.AgentMessage{msg})
}

// AppendMessages inserts messages transactionally; duplicates by id are
// ignored (INSERT OR IGNORE).
func (s *Service) AppendMessages(sessionID string, messages []types.AgentMessage) error {
	if len(messages) == 0 {
		return nil
	}
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	if !ok {
		slog.Warn("append messages: unknown session, messages not persisted",
			"session", sessionID, "count", len(messages))
		return nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}

	now := utils.NowMillis()
	images := s.imagesFor(sessionID, projectID)
	withContent := hasContentColumn(conn)
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	inserted := 0
	for _, msg := range messages {
		id := msg.ID
		if id == "" {
			sum := sha256.Sum256(msg.Data)
			id = hex.EncodeToString(sum[:])[:32]
		}
		added, err := insertMessage(tx, images, true, withContent, id, msg.Role, string(msg.Data), now)
		if err != nil {
			tx.Rollback()
			return err
		}
		if added {
			inserted++
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.bumpSessionUpdated(sessionID, now, inserted)
	return nil
}

// ReplaceMessages rewrites session history after repairing an interrupted
// tool turn.
func (s *Service) ReplaceMessages(sessionID string, messages []types.AgentMessage) error {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}

	now := utils.NowMillis()
	images := s.imagesFor(sessionID, projectID)
	withContent := hasContentColumn(conn)
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	// Delete the parts explicitly rather than relying on ON DELETE CASCADE,
	// which only fires when the connection has foreign_keys switched on.
	if _, err := tx.Exec(`DELETE FROM message_parts`); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages`); err != nil {
		tx.Rollback()
		return err
	}
	for i, msg := range messages {
		id := msg.ID
		if id == "" {
			sum := sha256.Sum256([]byte(jsonEncodeMessage(i, msg.Data)))
			id = hex.EncodeToString(sum[:])
		}
		if _, err := insertMessage(tx, images, false, withContent, id, msg.Role, string(msg.Data), now); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Images the old history held may no longer be referenced by anything.
	s.sweepImages(sessionID, projectID, conn)
	s.bumpSessionUpdated(sessionID, now, len(messages))
	return nil
}
