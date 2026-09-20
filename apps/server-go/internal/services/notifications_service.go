// Notification bus: services push events, the SSE route fans them out.
// Port of apps/server/api/src/services/notification.service.ts.
package services

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type NotificationService struct {
	mu   sync.Mutex
	subs map[chan types.NotificationEvent]bool
}

func NewNotificationService() *NotificationService {
	return &NotificationService{subs: make(map[chan types.NotificationEvent]bool)}
}

func (s *NotificationService) Push(event types.NotificationEvent) {
	s.mu.Lock()
	subs := make([]chan types.NotificationEvent, 0, len(s.subs))
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

func (s *NotificationService) Subscribe() chan types.NotificationEvent {
	ch := make(chan types.NotificationEvent, 64)
	s.mu.Lock()
	s.subs[ch] = true
	s.mu.Unlock()
	return ch
}

func (s *NotificationService) Unsubscribe(ch chan types.NotificationEvent) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}
