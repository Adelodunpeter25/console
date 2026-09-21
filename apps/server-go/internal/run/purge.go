// Deleted-chat retention purge. Mirrors the TS purgeExpiredDeletedSessions:
// permanently remove soft-deleted sessions past retention, skipping runs
// in flight (deferred to a later sweep).
package run

import (
	"log/slog"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// DeletedSessionRetention is how long soft-deleted chats stay restorable
// (TS DELETED_SESSION_RETENTION_MS parity: 7 days).
const DeletedSessionRetention = 7 * 24 * time.Hour

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
				purged = append(purged, id)
			}
		}()
	}
	return purged
}
