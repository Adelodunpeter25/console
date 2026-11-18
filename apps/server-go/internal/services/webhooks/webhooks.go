// Webhooks: generic event subscriptions an agent creates for its own session
// through console.webhooks.*. Rows live in the global DB so the public ingress
// route can match incoming events across all sessions and survive restarts.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusDone    = "done"
	StatusExpired = "expired"
	StatusError   = "error"
)

var (
	ErrNotFound     = errors.New("webhook subscription not found")
	ErrUnauthorized = errors.New("webhook authentication failed")
	validTopic      = regexp.MustCompile("^[A-Za-z0-9._-]+$")
)

// Subscription is one generic webhook trigger bound to a session.
type Subscription struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"sessionId"`
	Topic       string         `json:"topic"`
	Secret      string         `json:"secret"`
	Filter      map[string]any `json:"filter,omitempty"`
	Prompt      string         `json:"prompt"`
	Once        bool           `json:"once"`
	Status      string         `json:"status"`
	ExpiresAt   int64          `json:"expiresAt,omitempty"`
	LastFiredAt int64          `json:"lastFiredAt,omitempty"`
	FireCount   int64          `json:"fireCount"`
	LastError   string         `json:"lastError,omitempty"`
	CreatedAt   int64          `json:"createdAt"`
}

// Event is emitted whenever a webhook subscription changes state.
type Event struct {
	Type         string        `json:"type"`
	Subscription *Subscription `json:"subscription,omitempty"`
	ID           string        `json:"id,omitempty"`
	SessionID    string        `json:"sessionId,omitempty"`
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

// Match finds active subscriptions for topic (and optional subID) whose secret
// verifies the request and whose filter matches the JSON payload.
//
// When subID is given:
//   - unknown id or mismatched topic returns ErrNotFound
//   - inactive/expired subscription returns nil, nil (ignored)
//   - failed auth returns ErrUnauthorized
func (s *Service) Match(topic, subID string, headers map[string]string, queryToken string, body []byte, now time.Time) ([]Subscription, error) {
	var candidates []Subscription
	if subID != "" {
		sub, err := s.Get(subID)
		if err != nil {
			return nil, err
		}
		if sub == nil || (topic != "" && sub.Topic != topic) {
			return nil, ErrNotFound
		}
		if sub.Status != StatusActive {
			return nil, nil
		}
		if sub.ExpiresAt > 0 && sub.ExpiresAt <= now.Unix() {
			_, _ = s.SetStatus(sub.ID, StatusExpired)
			return nil, nil
		}
		if !verifyAuth(sub.Secret, headers, queryToken, body) {
			return nil, ErrUnauthorized
		}
		candidates = []Subscription{*sub}
	} else {
		rows, err := s.db.Query(
			`SELECT `+columns+` FROM webhook_subscriptions WHERE topic = ? AND status = 'active' ORDER BY created_at`,
			topic,
		)
		if err != nil {
			return nil, err
		}
		all, err := scanRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		for _, sub := range all {
			if sub.ExpiresAt > 0 && sub.ExpiresAt <= now.Unix() {
				_, _ = s.SetStatus(sub.ID, StatusExpired)
				continue
			}
			if verifyAuth(sub.Secret, headers, queryToken, body) {
				candidates = append(candidates, sub)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	var payload any
	if len(strings.TrimSpace(string(body))) > 0 {
		_ = json.Unmarshal(body, &payload)
	}

	var matched []Subscription
	for _, sub := range candidates {
		if matchFilter(sub.Filter, payload) {
			matched = append(matched, sub)
		}
	}
	return matched, nil
}

// AfterFire records a delivery attempt. A one-shot subscription flips to done
// on success; a fire error marks the row error so broken sessions do not keep
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
	_, err := s.db.Exec(
		`UPDATE webhook_subscriptions
		 SET status = CASE once WHEN 1 THEN 'done' ELSE status END,
		     last_fired_at = ?,
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

func verifyAuth(secret string, headers map[string]string, queryToken string, body []byte) bool {
	if secret == "" {
		return false
	}
	if queryToken != "" && subtle.ConstantTimeCompare([]byte(queryToken), []byte(secret)) == 1 {
		return true
	}
	for k, v := range headers {
		switch strings.ToLower(k) {
		case "x-webhook-secret":
			if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(v)), []byte(secret)) == 1 {
				return true
			}
		case "authorization":
			if scheme, token, ok := strings.Cut(strings.TrimSpace(v), " "); ok && strings.EqualFold(scheme, "bearer") {
				if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(secret)) == 1 {
					return true
				}
			}
		case "x-hub-signature-256", "x-webhook-signature":
			if verifyHMACSHA256(secret, v, body) {
				return true
			}
		}
	}
	return false
}

func verifyHMACSHA256(secret, header string, body []byte) bool {
	sigHex := strings.TrimSpace(header)
	if strings.HasPrefix(strings.ToLower(sigHex), "sha256=") {
		sigHex = sigHex[len("sha256="):]
	}
	got, err := hex.DecodeString(sigHex)
	if err != nil || len(got) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(got, expected)
}

func matchFilter(filter map[string]any, payload any) bool {
	if len(filter) == 0 {
		return true
	}
	root, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	for path, expected := range filter {
		actual, found := lookupPath(root, path)
		if !found || !matchValue(expected, actual) {
			return false
		}
	}
	return true
}

func lookupPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

func matchValue(expected, actual any) bool {
	if list, ok := expected.([]any); ok {
		for _, item := range list {
			if scalarEqual(item, actual) {
				return true
			}
		}
		return false
	}
	return scalarEqual(expected, actual)
}

func scalarEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if fa, okA := toFloat(a); okA {
		if fb, okB := toFloat(b); okB {
			return fa == fb
		}
	}
	if ba, okA := a.(bool); okA {
		if bb, okB := b.(bool); okB {
			return ba == bb
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := strconv.ParseFloat(n.String(), 64)
		return f, err == nil
	}
	return 0, false
}
