// Run event hub: broadcast to live subscribers with a bounded ring for
// ?since= replay. Extracted from run.go; the Service drives runs, the Hub
// only moves events.
package run

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// Terminal outcomes broadcast when a run settles.
const (
	OutcomeDone    = "done"
	OutcomeAborted = "aborted"
)

// Frame is one sequenced hub event for attaching clients.
type Frame struct {
	Seq   int64
	Event loop.Event
}

// Hub broadcasts run events to live subscribers and keeps a bounded ring
// for ?since= replay. Each subscriber has its own unbounded queue drained
// by a pump goroutine, so a slow client never stalls the run and never
// silently loses frames (TS event-stream parity). Only a client that falls
// subMaxBacklog frames behind — i.e. is effectively dead — is dropped.
type Hub struct {
	mu     sync.Mutex
	seq    int64
	buf    []Frame
	subs   map[string]*subscriber
	closed bool
	done   chan struct{}
	// Outcome is set by Close: "done" or "aborted".
	Outcome string
	// lastAt is the last Broadcast time; the run watchdog uses it to spot
	// turns that went silent.
	lastAt time.Time
}

const hubBuffer = 500

// subMaxBacklog bounds one subscriber's pending queue. Far above any
// legitimate burst; reaching it means the client stopped reading.
const subMaxBacklog = 50000

type subscriber struct {
	mu      sync.Mutex
	queue   []Frame
	closing bool // no more frames; drain queue then close out
	stopped bool // unsubscribed; close out immediately
	wake    chan struct{}
	out     chan Frame
}

func newSubscriber() *subscriber {
	s := &subscriber{wake: make(chan struct{}, 1), out: make(chan Frame)}
	go s.pump()
	return s
}

func (s *subscriber) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// push enqueues a frame; false when the backlog cap is exceeded.
func (s *subscriber) push(f Frame) bool {
	s.mu.Lock()
	if len(s.queue) >= subMaxBacklog {
		s.mu.Unlock()
		return false
	}
	s.queue = append(s.queue, f)
	s.mu.Unlock()
	s.signal()
	return true
}

// finish lets the pump deliver what is queued, then close out.
func (s *subscriber) finish() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.signal()
}

// stop closes out without delivering the remaining queue.
func (s *subscriber) stop() {
	s.mu.Lock()
	s.stopped = true
	s.queue = nil
	s.mu.Unlock()
	s.signal()
}

func (s *subscriber) pump() {
	defer close(s.out)
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		if len(s.queue) == 0 {
			closing := s.closing
			s.mu.Unlock()
			if closing {
				return
			}
			<-s.wake
			continue
		}
		f := s.queue[0]
		s.queue[0] = Frame{}
		s.queue = s.queue[1:]
		s.mu.Unlock()
		for sent := false; !sent; {
			select {
			case s.out <- f:
				sent = true
			case <-s.wake:
				s.mu.Lock()
				stopped := s.stopped
				s.mu.Unlock()
				if stopped {
					return
				}
			}
		}
	}
}

// NewHub creates a broadcast hub (exported for tests and attach flows).
func NewHub() *Hub {
	return &Hub{subs: map[string]*subscriber{}, done: make(chan struct{}), lastAt: time.Now()}
}

// Broadcast assigns the next sequence number and delivers to subscribers.
func (h *Hub) Broadcast(event loop.Event) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.seq++
	h.lastAt = time.Now()
	frame := Frame{Seq: h.seq, Event: event}
	h.buf = append(h.buf, frame)
	if len(h.buf) > hubBuffer {
		h.buf = h.buf[len(h.buf)-hubBuffer:]
	}
	for id, sub := range h.subs {
		if !sub.push(frame) {
			slog.Warn("run hub: subscriber stopped reading, dropping", "sub", id)
			delete(h.subs, id)
			sub.stop()
		}
	}
	h.mu.Unlock()
}

// Subscribe registers a live subscriber. When since != nil the buffered
// frames newer than since are returned for replay first.
func (h *Hub) Subscribe(since *int64) (id string, ch <-chan Frame, replay []Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return "", nil, nil
	}
	id = newSubID()
	sub := newSubscriber()
	h.subs[id] = sub
	if since != nil {
		for _, f := range h.buf {
			if f.Seq > *since {
				replay = append(replay, f)
			}
		}
	}
	return id, sub.out, replay
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *Hub) Unsubscribe(id string) {
	h.mu.Lock()
	if sub, ok := h.subs[id]; ok {
		delete(h.subs, id)
		sub.stop()
	}
	h.mu.Unlock()
}

// Close terminates the hub with an outcome, waking settle waiters. Live
// subscribers still receive every queued frame (incl. sessionEnd) before
// their channel closes.
func (h *Hub) Close(outcome string) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.Outcome = outcome
	for id, sub := range h.subs {
		delete(h.subs, id)
		sub.finish()
	}
	close(h.done)
	h.mu.Unlock()
}

// Done closes when the run settles.
func (h *Hub) Done() <-chan struct{} { return h.done }

// IdleSince reports how long ago the last event was broadcast. The run
// watchdog uses it to spot turns that went silent.
func (h *Hub) IdleSince() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.lastAt)
}

func newSubID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "sub_" + hex.EncodeToString(b)
}
