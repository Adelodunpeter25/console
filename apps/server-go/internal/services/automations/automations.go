// Automations: schedules an agent creates for its own session through the
// console.automation.* tools. One active automation per session; rows live in
// the global DB so the scanner sweeps due work in one query and schedules
// survive server restarts.
package automations

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	KindOnce     = "once"
	KindInterval = "interval"

	StatusActive = "active"
	StatusPaused = "paused"
	StatusDone   = "done"
	StatusError  = "error"
)

// ErrActiveExists rejects a second active automation on one session.
var ErrActiveExists = errors.New("session already has an active automation")

// Automation is one schedule row.
type Automation struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	Prompt          string `json:"prompt"`
	Kind            string `json:"kind"`
	IntervalSeconds int64  `json:"intervalSeconds,omitempty"`
	// RunAt is the absolute fire time of a 'once' automation (unix seconds).
	RunAt int64 `json:"runAt,omitempty"`
	// NextRunAt is when the scanner fires next (unix seconds).
	NextRunAt   int64  `json:"nextRunAt"`
	Status      string `json:"status"`
	LastFiredAt int64  `json:"lastFiredAt,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

// Event is emitted whenever an automation changes state.
type Event struct {
	Type       string      `json:"type"`
	Automation *Automation `json:"automation,omitempty"`
	ID         string      `json:"id,omitempty"`
	SessionID  string      `json:"sessionId,omitempty"`
}

type Service struct {
	db   *sql.DB
	mu   sync.Mutex
	subs map[chan Event]bool
}

func New(db *sql.DB) *Service {
	return &Service{db: db, subs: make(map[chan Event]bool)}
}

func (s *Service) Subscribe() chan Event {
	ch := make(chan Event, 64)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan Event]bool)
	}
	s.subs[ch] = true
	s.mu.Unlock()
	return ch
}

func (s *Service) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *Service) emit(ev Event) {
	s.mu.Lock()
	subs := make([]chan Event, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

const columns = `id, session_id, prompt, kind, interval_seconds, run_at, next_run_at, status, last_fired_at, last_error, created_at`

func scanAutomation(row interface{ Scan(...any) error }) (*Automation, error) {
	var a Automation
	var interval, runAt, lastFired sql.NullInt64
	var lastError sql.NullString
	if err := row.Scan(&a.ID, &a.SessionID, &a.Prompt, &a.Kind, &interval, &runAt,
		&a.NextRunAt, &a.Status, &lastFired, &lastError, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.IntervalSeconds, a.RunAt, a.LastFiredAt = interval.Int64, runAt.Int64, lastFired.Int64
	a.LastError = lastError.String
	return &a, nil
}

// Create stores a new active automation and enforces one active per session.
// The uniqueness check and insert share one immediate transaction, so two
// concurrent creates cannot both win.
func (s *Service) Create(sessionID, prompt, kind string, intervalSeconds, runAt int64, now time.Time) (*Automation, error) {
	if prompt == "" {
		return nil, errors.New("automation prompt is required")
	}
	if kind != KindOnce && kind != KindInterval {
		return nil, errors.New("automation kind must be 'once' or 'interval'")
	}
	if kind == KindInterval && intervalSeconds <= 0 {
		return nil, errors.New("interval automations need a positive intervalSeconds")
	}
	if kind == KindOnce && runAt <= 0 {
		return nil, errors.New("once automations need a future runAt")
	}
	if kind == KindInterval {
		runAt = 0
	} else {
		intervalSeconds = 0
	}
	next := runAt
	if kind == KindInterval {
		next = now.Unix() + intervalSeconds
	}
	a := &Automation{
		ID: "auto_" + utils.RandomID(), SessionID: sessionID, Prompt: prompt,
		Kind: kind, IntervalSeconds: intervalSeconds, RunAt: runAt,
		NextRunAt: next, Status: StatusActive, CreatedAt: now.Unix(),
	}
	if _, err := s.db.Exec(
		`INSERT INTO automations (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		a.ID, a.SessionID, a.Prompt, a.Kind,
		nullable(a.IntervalSeconds > 0, a.IntervalSeconds), nullable(a.RunAt > 0, a.RunAt),
		a.NextRunAt, a.Status, a.CreatedAt); err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrActiveExists
		}
		return nil, err
	}
	s.emit(Event{Type: "created", Automation: a, ID: a.ID, SessionID: a.SessionID})
	return a, nil
}

func nullable(has bool, v int64) any {
	if has {
		return v
	}
	return nil
}

func (s *Service) Get(id string) (*Automation, error) {
	row := s.db.QueryRow(`SELECT `+columns+` FROM automations WHERE id = ?`, id)
	a, err := scanAutomation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// ActiveForSession returns the session's active automation, or nil.
func (s *Service) ActiveForSession(sessionID string) (*Automation, error) {
	row := s.db.QueryRow(
		`SELECT `+columns+` FROM automations WHERE session_id = ? AND status = 'active'`, sessionID)
	a, err := scanAutomation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (s *Service) ListAll() ([]Automation, error) {
	rows, err := s.db.Query(`SELECT ` + columns + ` FROM automations ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Service) ListForSession(sessionID string) ([]Automation, error) {
	rows, err := s.db.Query(
		`SELECT `+columns+` FROM automations WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]Automation, error) {
	var out []Automation
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Update rewrites the mutable fields of an active, paused, or errored
// automation and recomputes next_run_at from now. Updating an errored
// automation revives it to active. Returns (nil, nil) when the id is unknown or
// already done.
func (s *Service) Update(id, prompt, kind string, intervalSeconds, runAt int64, now time.Time) (*Automation, error) {
	if prompt == "" {
		return nil, errors.New("automation prompt is required")
	}
	if kind != KindOnce && kind != KindInterval {
		return nil, errors.New("automation kind must be 'once' or 'interval'")
	}
	if kind == KindInterval && intervalSeconds <= 0 {
		return nil, errors.New("interval automations need a positive intervalSeconds")
	}
	if kind == KindOnce && runAt <= 0 {
		return nil, errors.New("once automations need a future runAt")
	}
	if kind == KindInterval {
		runAt = 0
	} else {
		intervalSeconds = 0
	}
	next := runAt
	if kind == KindInterval {
		next = now.Unix() + intervalSeconds
	}
	res, err := s.db.Exec(
		`UPDATE automations
		 SET prompt = ?, kind = ?, interval_seconds = ?, run_at = ?, next_run_at = ?,
		     status = CASE status WHEN 'error' THEN 'active' ELSE status END,
		     last_error = NULL
		 WHERE id = ? AND status IN ('active', 'paused', 'error')`,
		prompt, kind, nullable(intervalSeconds > 0, intervalSeconds), nullable(runAt > 0, runAt), next, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrActiveExists
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	updated, err := s.Get(id)
	if err == nil && updated != nil {
		s.emit(Event{Type: "updated", Automation: updated, ID: updated.ID, SessionID: updated.SessionID})
	}
	return updated, err
}

// Pause pauses an active automation.
func (s *Service) Pause(id string) (*Automation, error) {
	res, err := s.db.Exec(`UPDATE automations SET status = 'paused' WHERE id = ? AND status = 'active'`, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	paused, err := s.Get(id)
	if err == nil && paused != nil {
		s.emit(Event{Type: "paused", Automation: paused, ID: paused.ID, SessionID: paused.SessionID})
	}
	return paused, err
}

// Resume activates a paused or errored automation, clearing any prior error and
// advancing stale interval schedules past now.
func (s *Service) Resume(id string, now time.Time) (*Automation, error) {
	cur, err := s.Get(id)
	if err != nil || cur == nil {
		return nil, err
	}
	if cur.Status != StatusPaused && cur.Status != StatusError {
		return nil, nil
	}
	next := cur.NextRunAt
	if cur.Kind == KindInterval && cur.IntervalSeconds > 0 && next <= now.Unix() {
		next = now.Unix() + cur.IntervalSeconds
	}
	res, err := s.db.Exec(
		`UPDATE automations SET status = 'active', next_run_at = ?, last_error = NULL
		 WHERE id = ? AND status IN ('paused', 'error')`,
		next, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrActiveExists
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	resumed, err := s.Get(id)
	if err == nil && resumed != nil {
		s.emit(Event{Type: "resumed", Automation: resumed, ID: resumed.ID, SessionID: resumed.SessionID})
	}
	return resumed, err
}

// SetStatus flips active/paused; done and error are terminal states the
// scanner owns.
func (s *Service) SetStatus(id, status string) (bool, error) {
	if status != StatusActive && status != StatusPaused && status != StatusDone && status != StatusError {
		return false, errors.New("invalid automation status")
	}
	res, err := s.db.Exec(`UPDATE automations SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Service) Delete(id string) (bool, error) {
	existing, _ := s.Get(id)
	res, err := s.db.Exec(`DELETE FROM automations WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		sessionID := ""
		if existing != nil {
			sessionID = existing.SessionID
		}
		s.emit(Event{Type: "deleted", ID: id, SessionID: sessionID})
	}
	return n > 0, nil
}

// DeleteForSession removes every automation of a session.
func (s *Service) DeleteForSession(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM automations WHERE session_id = ?`, sessionID)
	return err
}

// Due lists active automations whose fire time has passed, in fire order.
func (s *Service) Due(now time.Time) ([]Automation, error) {
	rows, err := s.db.Query(
		`SELECT `+columns+` FROM automations WHERE status = 'active' AND next_run_at <= ? ORDER BY next_run_at`,
		now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// AfterFire advances an interval automation to its next occurrence, or deletes
// a completed one-shot. A fire error marks the row error so a broken session
// cannot burn the scanner every tick; the agent can see and clear it.
func (s *Service) AfterFire(id string, fireErr error, now time.Time) error {
	if fireErr != nil {
		_, err := s.db.Exec(
			`UPDATE automations SET status = 'error', last_fired_at = ?, last_error = ? WHERE id = ?`,
			now.Unix(), fireErr.Error(), id)
		if err == nil {
			if updated, gerr := s.Get(id); gerr == nil && updated != nil {
				s.emit(Event{Type: "error", Automation: updated, ID: updated.ID, SessionID: updated.SessionID})
			}
		}
		return err
	}
	cur, err := s.Get(id)
	if err != nil || cur == nil {
		return err
	}
	if cur.Kind == KindOnce {
		if _, err := s.db.Exec(`DELETE FROM automations WHERE id = ?`, id); err != nil {
			return err
		}
		done := *cur
		done.Status = StatusDone
		done.LastFiredAt = now.Unix()
		done.LastError = ""
		s.emit(Event{Type: "fired", Automation: &done, ID: done.ID, SessionID: done.SessionID})
		return nil
	}
	next := cur.NextRunAt
	if cur.Kind == KindInterval && cur.IntervalSeconds > 0 {
		if now.Unix() < cur.NextRunAt {
			next = now.Unix() + cur.IntervalSeconds
		} else {
			// Anchor to the scheduled slot so scan lateness does not drift,
			// skipping missed slots rather than burst-firing catch-ups.
			steps := (now.Unix()-cur.NextRunAt)/cur.IntervalSeconds + 1
			next = cur.NextRunAt + steps*cur.IntervalSeconds
		}
	}
	_, err = s.db.Exec(
		`UPDATE automations SET last_fired_at = ?, next_run_at = ?, last_error = NULL WHERE id = ?`,
		now.Unix(), next, id)
	if err == nil {
		if updated, gerr := s.Get(id); gerr == nil && updated != nil {
			s.emit(Event{Type: "fired", Automation: updated, ID: updated.ID, SessionID: updated.SessionID})
		}
	}
	return err
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
