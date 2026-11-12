// Unread state: a chat is unread when a run finished or stopped to ask the
// user something after any client last showed the chat.
package session

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// IsUnread is the one definition of unread, derived from the two timestamps.
// A chat with no recorded activity is never unread, so rows from before the
// columns existed stay quiet after an upgrade. A running chat shows its
// spinner and a settled chat is filed away, so neither is unread.
func IsUnread(h types.SessionHeader) bool {
	if h.LastActivityAt == nil || h.SettledAt != nil || h.Status == "working" {
		return false
	}
	return h.LastReadAt == nil || *h.LastActivityAt > *h.LastReadAt
}

// MarkActivity records that a run finished or needs the user. It deliberately
// leaves updated_at alone: it is a notification marker, not a content change,
// and updated_at orders the sidebar.
func (s *Service) MarkActivity(sessionID string) error {
	_, err := s.manager.Global().Exec(
		`UPDATE sessions SET last_activity_at = ? WHERE id = ?`,
		utils.NowMillis(), sessionID)
	return err
}

// MarkRead records that a client showed the chat. A lingering needs_attention
// with no run behind it (an errored run, or a crash during a question) is
// cleared here: the user has now seen it, which is exactly what
// settle-on-read deliberately waits for. Reports whether the id is known.
func (s *Service) MarkRead(sessionID string) (bool, error) {
	now := utils.NowMillis()
	// last_read_at must not fall behind last_activity_at set in the same
	// millisecond, or a just-finished chat would read as unread right after
	// being read.
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET last_read_at = MAX(?, COALESCE(last_activity_at, 0)) WHERE id = ?`,
		now, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	if s.isBusy == nil || !s.isBusy(sessionID) {
		if _, err := s.manager.Global().Exec(
			`UPDATE sessions SET status = 'done' WHERE id = ? AND status = 'needs_attention'`,
			sessionID); err != nil {
			return true, err
		}
	}
	return true, nil
}

// MarkUnread makes a read chat unread again. It refuses a chat with a run in
// flight (it shows the spinner) and a settled one (it is filed away), via
// ErrSessionBusy and ErrSessionSettled. Activity is moved forward instead of
// clearing last_read_at, because a chat that has never run has no activity
// time to compare against.
func (s *Service) MarkUnread(sessionID string) (bool, error) {
	if s.isBusy != nil && s.isBusy(sessionID) {
		return false, ErrSessionBusy
	}
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET last_activity_at = MAX(?, COALESCE(last_read_at, 0) + 1)
		 WHERE id = ? AND settled_at IS NULL`,
		utils.NowMillis(), sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, nil
	}
	var settled bool
	if err := s.manager.Global().QueryRow(
		`SELECT settled_at IS NOT NULL FROM sessions WHERE id = ?`, sessionID,
	).Scan(&settled); err != nil {
		return false, nil
	}
	if settled {
		return false, ErrSessionSettled
	}
	return false, nil
}
