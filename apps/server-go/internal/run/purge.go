// Chat retention sweeps: soft-delete chats settled past retention, then
// permanently remove soft-deleted sessions past retention. Both skip runs in
// flight (deferred to a later sweep).
package run

import (
	"log/slog"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// DeletedSessionRetention is how long soft-deleted chats stay restorable
// (7 days).
const DeletedSessionRetention = 7 * 24 * time.Hour

// SettledSessionRetention is how long a settled chat is kept before the
// sweep soft-deletes it (7 days); it then stays restorable for
// DeletedSessionRetention.
const SettledSessionRetention = 7 * 24 * time.Hour

// DeleteExpiredSettledSessions soft-deletes chats settled longer than the
// retention window. Sessions with an active run are skipped. Best-effort per
// session: one failure never blocks the rest.
func (s *Service) DeleteExpiredSettledSessions() []string {
	cutoff := utils.NowMillis() - SettledSessionRetention.Milliseconds()
	ids, err := s.sessions.ExpiredSettledSessions(cutoff)
	if err != nil {
		slog.Error("settled-chat sweep query failed", "error", err)
		return nil
	}
	deleted := make([]string, 0, len(ids))
	for _, id := range ids {
		if s.IsActive(id) {
			continue
		}
		if ok, err := s.sessions.SoftDelete(id); err != nil {
			slog.Error("settled-chat sweep failed", "session", id, "error", err)
		} else if ok {
			s.NotifySessionDeleted(id)
			deleted = append(deleted, id)
		}
	}
	return deleted
}

// PurgeExpiredDeletedSessions permanently removes soft-deleted sessions
// older than the retention window. Sessions with an active run are skipped.
// Best-effort per session: one failure never blocks the rest.
func (s *Service) PurgeExpiredDeletedSessions() []string {
	cutoff := utils.NowMillis() - DeletedSessionRetention.Milliseconds()
	ids, err := s.sessions.ExpiredDeletedSessions(cutoff)
	if err != nil {
		slog.Error("deleted-chat sweep query failed", "error", err)
		return nil
	}
	purged := make([]string, 0, len(ids))
	for _, id := range ids {
		if s.IsActive(id) {
			continue
		}
		func() {
			defer func() {
				// PermanentDelete touches the filesystem; never let one
				// corrupt/locked session panic the sweep.
				if r := recover(); r != nil {
					slog.Error("deleted-chat sweep panicked", "session", id, "panic", r)
				}
			}()
			if ok, err := s.sessions.PermanentDelete(id); err != nil {
				slog.Error("deleted-chat sweep failed", "session", id, "error", err)
			} else if ok {
				// Clients close the chat's browser tabs on this signal; the
				// session is gone either way.
				s.NotifySessionDeleted(id)
				purged = append(purged, id)
			}
		}()
	}
	return purged
}
