// Notification bus: services push events, the SSE route fans them out.
package services

import (
	"sync"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

type NotificationService struct {
	mu   sync.Mutex
	subs map[chan *consolev1.NotificationEvent]bool
}

func NewNotificationService() *NotificationService {
	return &NotificationService{subs: make(map[chan *consolev1.NotificationEvent]bool)}
}

func (s *NotificationService) Push(event *consolev1.NotificationEvent) {
	s.mu.Lock()
	subs := make([]chan *consolev1.NotificationEvent, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *NotificationService) Subscribe() chan *consolev1.NotificationEvent {
	ch := make(chan *consolev1.NotificationEvent, 64)
	s.mu.Lock()
	s.subs[ch] = true
	s.mu.Unlock()
	return ch
}

func (s *NotificationService) Unsubscribe(ch chan *consolev1.NotificationEvent) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}
