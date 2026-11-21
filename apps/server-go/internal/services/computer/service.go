//go:build linux || darwin

package computer

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// FrameHandler receives raw wire frames (16-byte header + H.264) to stream to viewers.
type FrameHandler func(kind uint32, data []byte)

// Service manages the active desktop capture session and input forwarding.
type Service struct {
	mu          sync.Mutex
	displayMgr  *displayManager
	session     *voxlSession
	subscribers map[uint64]FrameHandler
	nextSubID   uint64
	running     bool
}

var (
	defaultService *Service
	once           sync.Once
)

// Default returns the singleton computer service instance.
func Default() *Service {
	once.Do(func() {
		defaultService = New()
	})
	return defaultService
}

// New creates a new Computer streaming service.
func New() *Service {
	return &Service{
		displayMgr:  newDisplayManager(),
		subscribers: make(map[uint64]FrameHandler),
	}
}

// Subscribe attaches a viewer to receive desktop video frames.
// If the capture session is not active, it initializes the display and starts voxl.
func (s *Service) Subscribe(ctx context.Context, handler FrameHandler) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		display, err := s.displayMgr.EnsureDisplay(ctx)
		if err != nil {
			return nil, fmt.Errorf("computer: failed to prepare display: %w", err)
		}

		sess, err := newVoxlSession(voxlConfig{
			Display:      display,
			Preset:       "ultrafast",
			FPS:          60,
			MaxKbps:      30000,
			MinKbps:      2000,
			EnableResize: true,
		})
		if err != nil {
			return nil, fmt.Errorf("computer: failed to create voxl session: %w", err)
		}

		err = sess.Start(func(kind uint32, data []byte) {
			s.mu.Lock()
			subs := make([]FrameHandler, 0, len(s.subscribers))
			for _, h := range s.subscribers {
				subs = append(subs, h)
			}
			s.mu.Unlock()

			for _, sub := range subs {
				sub(kind, data)
			}
		})
		if err != nil {
			sess.Close()
			return nil, fmt.Errorf("computer: failed to start voxl session: %w", err)
		}

		s.session = sess
		s.running = true
	}

	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = handler

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers, id)
		if len(s.subscribers) == 0 && s.running {
			_ = s.session.Stop()
			s.session.Close()
			s.session = nil
			s.running = false
			s.displayMgr.Close()
		}
	}

	return unsubscribe, nil
}

// SendMouse forwards mouse motion and button events to the desktop session.
func (s *Service) SendMouse(x, y int32, buttonMask uint32, wheel int32) error {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return errors.New("computer: no active session")
	}
	return sess.SendMouse(x, y, buttonMask, wheel)
}

// SendKey forwards a keypress (keysym and down state) to the desktop session.
func (s *Service) SendKey(keysym uint32, down bool) error {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return errors.New("computer: no active session")
	}
	return sess.SendKey(keysym, down)
}

// Resize requests a dynamic resolution change for the display.
func (s *Service) Resize(width, height uint32) error {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return errors.New("computer: no active session")
	}
	return sess.Resize(width, height)
}

// Close gracefully stops the streaming service and display manager.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		_ = s.session.Stop()
		s.session.Close()
		s.session = nil
	}
	s.running = false
	s.subscribers = make(map[uint64]FrameHandler)
	s.displayMgr.Close()
}