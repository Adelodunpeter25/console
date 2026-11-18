// Webhooks: generic event subscriptions an agent creates for its own session
// through console.webhooks.*. Rows live in the global DB so the public ingress
// route can match incoming events across all sessions and survive restarts.
package webhooks

import (
	"database/sql"
	"errors"
	"regexp"
	"sync"
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
