// Core session operations: create, list, load, soft delete.
package session

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ErrSessionBusy refuses a settle: the session has a run in flight or a
// queued prompt, so it is not finished work. The routes layer maps it to 409.
var ErrSessionBusy = errors.New("session has an active run or queued prompt")

// ErrCwdMissing rejects pointing a project session at a working directory
// that is no longer on disk (typically: the folder was deleted after the
// project was registered). Failing here beats a session whose every tool
// call and terminal spawn dies later. The routes layer maps it to 400.
var ErrCwdMissing = errors.New("working directory does not exist")

type Service struct {
	manager *db.DB
	// backfilled remembers sessions whose old messages are all converted, so the
	// check is skipped on every later load in this process.
	backfilled sync.Map
	// verified remembers which messages were already proven to rebuild from
	// their parts exactly, so each is checked once per process.
	verified verifiedSet
	// isBusy reports whether a run is in flight on a session. Wired by the
	// caller (the run service's IsActive) rather than imported, since run
	// already depends on this package.
	isBusy func(id string) bool
}

func New(manager *db.DB) *Service {
	return &Service{manager: manager}
}

// SetBusyCheck wires the run-in-flight probe used by Settle. Called once at
// startup, after the run service exists; nil (tests) means never busy.
func (s *Service) SetBusyCheck(fn func(id string) bool) { s.isBusy = fn }

// Fallback model and provider used when Create is not given one.
const (
	DefaultFallbackModel    = "claude-opus-4-6-thinking"
	DefaultFallbackProvider = "antigravity"
)

// Create resolves defaults (model/provider/title/cwd/project,
// scratchpad for explicit-null projectId) then writes the index + meta rows.
func (s *Service) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
	id := opts.ID
	// TitleFirst (not TrimSpace) so every stored title — created, generated,
	// or renamed — reads capitalized on desktop and mobile alike.
	title := utils.TitleFirst(opts.Title)
	if title == "" {
		title = "New Session"
	}
	modelID := opts.ModelID
	if modelID == "" {
		modelID = DefaultFallbackModel
	}
	provider := opts.Provider
	if provider == "" {
		provider = DefaultFallbackProvider
	}
	approvalMode := opts.ApprovalMode
	if approvalMode == "" {
		approvalMode = "always-ask"
	}

	cwd := opts.Cwd
	var projectID *string
	if opts.ProjectNull {
		// Explicit null => scratchpad session: fresh id, sandboxed cwd.
		id = utils.RandomID()
		if cwd == "" {
			cwd = filepath.Join(utils.ConsoleStorageDir(), "scratch", id)
		}
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			return types.SessionHeader{}, err
		}
	} else {
		if cwd == "" {
			if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}
		}
		if cwd != "" {
			if _, err := os.Stat(cwd); err != nil {
				return types.SessionHeader{}, fmt.Errorf("%w: %s", ErrCwdMissing, cwd)
			}
		}
		if opts.ProjectID != nil {
			projectID = opts.ProjectID
		} else if cwd != "" {
			if found, err := s.projectByDir(cwd); err == nil && found != "" {
				projectID = &found
			}
		}
	}
	if id == "" {
		id = utils.RandomID()
	}
	now := utils.NowMillis()

	if _, err := s.manager.Global().Exec(`
		INSERT INTO sessions
			(id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, thinking_level, created_at, updated_at,
				worktree_path, worktree_branch, worktree_repo)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'idle', ?, ?, ?, ?,
			?, ?, ?)`,
		id, title, cwd, projectID, modelID, provider, approvalMode, opts.ThinkingLevel, now, now,
		worktreeCol(opts.ResolvedWorktree, func(w *types.SessionWorktree) string { return w.Path }),
		worktreeCol(opts.ResolvedWorktree, func(w *types.SessionWorktree) string { return w.Branch }),
		worktreeCol(opts.ResolvedWorktree, func(w *types.SessionWorktree) string { return w.Repo }),
	); err != nil {
		return types.SessionHeader{}, err
	}

	conn, err := s.manager.Session(id, derefString(projectID))
	if err != nil {
		return types.SessionHeader{}, err
	}
	if _, err := conn.Exec(`
		INSERT INTO session_meta
			(id, title, cwd, project_id, model_id, provider, approval_mode, thinking_level, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, cwd, projectID, modelID, provider, approvalMode, opts.ThinkingLevel, now, now,
	); err != nil {
		return types.SessionHeader{}, err
	}

	return types.SessionHeader{
		ID: id, Title: title, Cwd: cwd, ProjectID: projectID,
		ModelID: modelID, Provider: provider, ApprovalMode: approvalMode,
		ThinkingLevel: opts.ThinkingLevel,
		CreatedAt:     now, UpdatedAt: now, MessageCount: 0, Status: "idle",
		Worktree: opts.ResolvedWorktree,
	}, nil
}

// worktreeCol extracts one worktree column for INSERT, or nil (NULL) when
// the session owns no worktree.
func worktreeCol(w *types.SessionWorktree, pick func(*types.SessionWorktree) string) any {
	if w == nil {
		return nil
	}
	return pick(w)
}

// ListFilter holds the options for ListFiltered.
type ListFilter struct {
	Cwd         string
	ProjectID   string
	OnlyDeleted bool
}

const sessionColumns = `id, title, cwd, project_id, model_id, provider, approval_mode,
	thinking_level, pinned_at, settled_at, created_at, updated_at, message_count, status, deleted_at,
	worktree_path, worktree_branch, worktree_repo`

const sessionListOrder = `ORDER BY pinned_at IS NULL, pinned_at DESC, updated_at DESC`

// ListFiltered lists sessions: limit 100, pinned first (most recently
// pinned), then newest first. Cwd takes precedence over projectId, deleted
// filter flips the condition.
func (s *Service) ListFiltered(f ListFilter) ([]types.SessionHeader, error) {
	const limit = 100
	deletedCondition := "deleted_at IS NULL"
	if f.OnlyDeleted {
		deletedCondition = "deleted_at IS NOT NULL"
	}
	var rows *sql.Rows
	var err error
	switch {
	case f.Cwd != "":
		rows, err = s.manager.Global().Query(`
			SELECT `+sessionColumns+`
			FROM sessions WHERE cwd = ? AND `+deletedCondition+` `+sessionListOrder+` LIMIT ?`, f.Cwd, limit)
	case f.ProjectID != "":
		rows, err = s.manager.Global().Query(`
			SELECT `+sessionColumns+`
			FROM sessions WHERE project_id = ? AND `+deletedCondition+` `+sessionListOrder+` LIMIT ?`, f.ProjectID, limit)
	default:
		rows, err = s.manager.Global().Query(`
			SELECT `+sessionColumns+`
			FROM sessions WHERE `+deletedCondition+` `+sessionListOrder+` LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionRows(rows)
}

// OwnedWorktreePaths lists every worktree_path claimed by a session row
// (including soft-deleted ones — trash still owns its worktree). Used for
// orphan detection: dirs under the worktree root with no owner are orphans.
func (s *Service) OwnedWorktreePaths() ([]string, error) {
	rows, err := s.manager.Global().Query(
		`SELECT DISTINCT worktree_path FROM sessions WHERE worktree_path IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func scanSessionRows(rows *sql.Rows) ([]types.SessionHeader, error) {
	out := make([]types.SessionHeader, 0)
	for rows.Next() {
		var h types.SessionHeader
		var projectID sql.NullString
		var thinkingLevel sql.NullString
		var pinnedAt sql.NullInt64
		var settledAt sql.NullInt64
		var deletedAt sql.NullInt64
		var wtPath, wtBranch, wtRepo sql.NullString
		if err := rows.Scan(&h.ID, &h.Title, &h.Cwd, &projectID, &h.ModelID, &h.Provider,
			&h.ApprovalMode, &thinkingLevel, &pinnedAt, &settledAt, &h.CreatedAt, &h.UpdatedAt, &h.MessageCount, &h.Status, &deletedAt,
			&wtPath, &wtBranch, &wtRepo); err != nil {
			return nil, err
		}
		if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
			id := projectID.String
			h.ProjectID = &id
		}
		if thinkingLevel.Valid && thinkingLevel.String != "" {
			v := thinkingLevel.String
			h.ThinkingLevel = &v
		}
		if pinnedAt.Valid {
			v := pinnedAt.Int64
			h.PinnedAt = &v
		}
		if deletedAt.Valid {
			v := deletedAt.Int64
			h.DeletedAt = &v
		}
		if settledAt.Valid {
			v := settledAt.Int64
			h.SettledAt = &v
		}
		if wtPath.Valid && wtPath.String != "" {
			h.Worktree = &types.SessionWorktree{
				Path:   wtPath.String,
				Branch: wtBranch.String,
				Repo:   wtRepo.String,
			}
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Load reads the header from the global index and history from the
// per-session DB. Pagination uses a rowid cursor,
// newest page first, limit <= 0 means all messages. Messages are the stored
// content JSON with createdAt injected, exactly as the desktop parses them.
func (s *Service) Load(sessionID string, limit int64, before int64) (*types.LoadedSession, error) {
	rows, err := s.manager.Global().Query(`
		SELECT `+sessionColumns+`
		FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	headers, err := scanSessionRows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 || headers[0].DeletedAt != nil {
		return nil, nil
	}

	conn, err := s.manager.Session(sessionID, projectIDFrom(headers[0].ProjectID))
	if err != nil {
		return nil, err
	}
	s.ensureBackfilled(sessionID, projectIDFrom(headers[0].ProjectID), conn)

	query := `SELECT content, created_at, rowid, id, skeleton, split_state FROM messages`
	var args []any
	if before > 0 {
		query += ` WHERE rowid < ?`
		args = append(args, before)
	}
	query += ` ORDER BY rowid DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}
	msgRows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	fetched := make([]storedRow, 0)
	for msgRows.Next() {
		var r storedRow
		if err := msgRows.Scan(&r.content, &r.createdAt, &r.rowid, &r.id, &r.skeleton, &r.state); err != nil {
			msgRows.Close()
			return nil, err
		}
		fetched = append(fetched, r)
	}
	err = msgRows.Err()
	// Close before reading parts: the session database has one connection, so a
	// second query while these rows are open would wait on itself forever.
	msgRows.Close()
	if err != nil {
		return nil, err
	}

	hasMore := limit > 0 && int64(len(fetched)) > limit
	if hasMore {
		fetched = fetched[:limit]
	}
	// Oldest first for display. Split messages are rebuilt from their parts and
	// checked against the original; anything unproven is served as stored.
	bodies := s.materialize(sessionID, projectIDFrom(headers[0].ProjectID), conn, fetched)
	messages := make([]json.RawMessage, 0, len(fetched))
	for i := len(fetched) - 1; i >= 0; i-- {
		messages = append(messages, withCreatedAt(bodies[i], fetched[i].createdAt, fetched[i].id))
	}
	result := &types.LoadedSession{Header: headers[0], Messages: messages, HasMore: hasMore}
	if hasMore && len(fetched) > 0 {
		cursor := fetched[len(fetched)-1].rowid
		result.NextCursor = &cursor
	}
	return result, nil
}

// withCreatedAt injects createdAt (the row's created_at) into a stored
// message object. Non-object payloads pass through untouched.
//
// Timestamps inject as protojson strings and the row id as a string: the
// canonical shape carries both on the wrapper, absent at persist time. The
// client keys composer history recall off a user message's id so it can
// re-stage that message's image attachments; the stored payload carries
// only the role-specific fields, so without this every loaded message
// would arrive id-less.
func withCreatedAt(content string, createdAt int64, id string) json.RawMessage {
	var obj map[string]any
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		return json.RawMessage(content)
	}
	obj["createdAt"] = strconv.FormatInt(createdAt, 10)
	if id != "" {
		obj["id"] = id
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(content)
	}
	return out
}

// SoftDelete marks a session deleted; retention purge is 7 days. It
// succeeds unconditionally and idempotently for known ids.
func (s *Service) SoftDelete(sessionID string) (bool, error) {
	now := utils.NowMillis()
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Settle marks a session settled: finished enough to sit in the collapsed
// Settled section, but kept. Idempotent — an already-settled session keeps
// its original settled_at, so its position in the section does not jump — and
// settling does not touch updated_at, because settling is not activity. A
// session with a run in flight or a queued prompt is refused with
// ErrSessionBusy: settling is for finished work.
func (s *Service) Settle(sessionID string) (bool, error) {
	if s.isBusy != nil && s.isBusy(sessionID) {
		return false, ErrSessionBusy
	}
	if qp, err := s.GetQueuedPrompt(sessionID); err == nil && qp != nil {
		return false, ErrSessionBusy
	}
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET settled_at = COALESCE(settled_at, ?) WHERE id = ?`,
		utils.NowMillis(), sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Unsettle clears the settled mark. It succeeds for any known id, even one
// that was not settled, and has no busy limit: a settled chat is idle by
// definition.
func (s *Service) Unsettle(sessionID string) (bool, error) {
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET settled_at = NULL WHERE id = ?`, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateTitle sets the session title in the global index and the
// per-session meta row. The incoming title is normalized like Create's, so
// a rename can't reintroduce a lowercase first letter.
func (s *Service) UpdateTitle(sessionID, title string) error {
	title = utils.TitleFirst(title)
	// A stored title is never empty (same default as Create), so clients
	// render it as-is with no fallback of their own.
	if title == "" {
		title = "New Session"
	}
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
	if _, err := conn.Exec(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`, title, now); err != nil {
		return err
	}
	if _, err := s.manager.Global().Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`, title, now, sessionID); err != nil {
		return err
	}
	return nil
}
