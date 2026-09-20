// Generic queue-based event stream. Port of
// apps/server/agent/src/service/event-stream.ts: events are always enqueued
// so a slow consumer never skips frames; Close(err) terminates with a
// terminal value extracted by the caller from the final event.
package stream

import "sync"

type Stream[T any] struct {
	mu       sync.Mutex
	events   []T
	waiters  []chan struct{}
	done     bool
	failed   error
	idx      int
	closedCh chan struct{}
}

func New[T any]() *Stream[T] {
	return &Stream[T]{closedCh: make(chan struct{})}
}

// Push appends an event and wakes consumers.
func (s *Stream[T]) Push(event T) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.events = append(s.events, event)
	for _, w := range s.waiters {
		close(w)
	}
	s.waiters = nil
	s.mu.Unlock()
}

// Fail terminates the stream with an error.
func (s *Stream[T]) Fail(err error) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.failed = err
	close(s.closedCh)
	s.mu.Unlock()
}

// Complete terminates the stream normally.
func (s *Stream[T]) Complete() {
	s.mu.Lock()
	if !s.done {
		s.done = true
		close(s.closedCh)
	}
	s.mu.Unlock()
}

// Next returns the next event; ok=false when the stream is drained or
// failed. err is non-nil only when Fail was called.
func (s *Stream[T]) Next() (event T, err error, ok bool) {
	for {
		s.mu.Lock()
		if s.idx < len(s.events) {
			event = s.events[s.idx]
			s.idx++
			s.mu.Unlock()
			return event, nil, true
		}
		if s.done {
			err = s.failed
			s.mu.Unlock()
			return event, err, false
		}
		waiter := make(chan struct{})
		s.waiters = append(s.waiters, waiter)
		s.mu.Unlock()

		<-waiter
	}
}
