package webhooks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const columns = `id, session_id, topic, secret, filter_json, prompt, once, status, expires_at, last_fired_at, fire_count, last_error, created_at`

func scanSubscription(row interface{ Scan(...any) error }) (*Subscription, error) {
	var s Subscription
	var filterJSON string
	var onceInt int
	var expiresAt, lastFired sql.NullInt64
	var lastError sql.NullString
	if err := row.Scan(
		&s.ID, &s.SessionID, &s.Topic, &s.Secret, &filterJSON, &s.Prompt,
		&onceInt, &s.Status, &expiresAt, &lastFired, &s.FireCount, &lastError, &s.CreatedAt,
	); err != nil {
		return nil, err
	}
	s.Once = onceInt != 0
	s.ExpiresAt = expiresAt.Int64
	s.LastFiredAt = lastFired.Int64
	s.LastError = lastError.String
	if filterJSON != "" && filterJSON != "{}" {
		if err := json.Unmarshal([]byte(filterJSON), &s.Filter); err != nil {
			return nil, fmt.Errorf("decode filter_json: %w", err)
		}
	}
	return &s, nil
}

func scanRows(rows *sql.Rows) ([]Subscription, error) {
	var out []Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Create stores a new active webhook subscription. If secret is empty, a random
// secret is generated.
func (s *Service) Create(sessionID, topic, prompt, secret string, filter map[string]any, once bool, expiresAt int64, now time.Time) (*Subscription, error) {
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	topic = strings.TrimSpace(topic)
	if !validTopic.MatchString(topic) {
		return nil, errors.New("topic must match [A-Za-z0-9._-]+")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("webhook prompt is required")
	}
	if expiresAt > 0 && expiresAt <= now.Unix() {
		return nil, errors.New("expiresAt must be in the future")
	}
	if secret == "" {
		secret = utils.RandomID() + utils.RandomID()
	}
	filterJSON := "{}"
	if len(filter) > 0 {
		raw, err := json.Marshal(filter)
		if err != nil {
			return nil, fmt.Errorf("encode filter: %w", err)
		}
		filterJSON = string(raw)
	}
	onceInt := 0
	if once {
		onceInt = 1
	}
	sub := &Subscription{
		ID:        "sub_" + utils.RandomID(),
		SessionID: sessionID,
		Topic:     topic,
		Secret:    secret,
		Filter:    filter,
		Prompt:    prompt,
		Once:      once,
		Status:    StatusActive,
		ExpiresAt: expiresAt,
		CreatedAt: now.Unix(),
	}
	var exp any
	if expiresAt > 0 {
		exp = expiresAt
	}
	if _, err := s.db.Exec(
		`INSERT INTO webhook_subscriptions (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 0, NULL, ?)`,
		sub.ID, sub.SessionID, sub.Topic, sub.Secret, filterJSON, sub.Prompt,
		onceInt, sub.Status, exp, sub.CreatedAt,
	); err != nil {
		return nil, err
	}
	s.emit(Event{Type: "created", Subscription: sub, ID: sub.ID, SessionID: sub.SessionID})
	return sub, nil
}

func (s *Service) Get(id string) (*Subscription, error) {
	row := s.db.QueryRow(`SELECT `+columns+` FROM webhook_subscriptions WHERE id = ?`, id)
	sub, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return sub, err
}

func (s *Service) ListAll() ([]Subscription, error) {
	rows, err := s.db.Query(`SELECT ` + columns + ` FROM webhook_subscriptions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Service) ListForSession(sessionID string) ([]Subscription, error) {
	rows, err := s.db.Query(
		`SELECT `+columns+` FROM webhook_subscriptions WHERE session_id = ? ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Service) SetStatus(id, status string) (bool, error) {
	switch status {
	case StatusActive, StatusPaused, StatusDone, StatusExpired, StatusError:
	default:
		return false, errors.New("invalid webhook subscription status")
	}
	res, err := s.db.Exec(`UPDATE webhook_subscriptions SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if updated, gerr := s.Get(id); gerr == nil && updated != nil {
			s.emit(Event{Type: status, Subscription: updated, ID: updated.ID, SessionID: updated.SessionID})
		}
	}
	return n > 0, nil
}

func (s *Service) Delete(id string) (bool, error) {
	existing, _ := s.Get(id)
	res, err := s.db.Exec(`DELETE FROM webhook_subscriptions WHERE id = ?`, id)
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

func (s *Service) DeleteForSession(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM webhook_subscriptions WHERE session_id = ?`, sessionID)
	return err
}

// AfterFire records a delivery attempt. A one-shot subscription is deleted on
// success; a fire error marks the row error so broken sessions do not keep
// failing silently.
func (s *Service) AfterFire(id string, fireErr error, now time.Time) error {
	if fireErr != nil {
		_, err := s.db.Exec(
			`UPDATE webhook_subscriptions SET status = 'error', last_fired_at = ?, fire_count = fire_count + 1, last_error = ? WHERE id = ?`,
			now.Unix(), fireErr.Error(), id,
		)
		if err == nil {
			if updated, gerr := s.Get(id); gerr == nil && updated != nil {
				s.emit(Event{Type: "error", Subscription: updated, ID: updated.ID, SessionID: updated.SessionID})
			}
		}
		return err
	}
	cur, err := s.Get(id)
	if err != nil || cur == nil {
		return err
	}
	if cur.Once {
		if _, err := s.db.Exec(`DELETE FROM webhook_subscriptions WHERE id = ?`, id); err != nil {
			return err
		}
		done := *cur
		done.Status = StatusDone
		done.LastFiredAt = now.Unix()
		done.FireCount++
		done.LastError = ""
		s.emit(Event{Type: "fired", Subscription: &done, ID: done.ID, SessionID: done.SessionID})
		return nil
	}
	_, err = s.db.Exec(
		`UPDATE webhook_subscriptions
		 SET last_fired_at = ?,
		     fire_count = fire_count + 1,
		     last_error = NULL
		 WHERE id = ?`,
		now.Unix(), id,
	)
	if err == nil {
		if updated, gerr := s.Get(id); gerr == nil && updated != nil {
			s.emit(Event{Type: "fired", Subscription: updated, ID: updated.ID, SessionID: updated.SessionID})
		}
	}
	return err
}
