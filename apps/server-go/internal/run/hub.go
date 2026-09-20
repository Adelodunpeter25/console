// Run event hub: broadcast to live subscribers with a bounded ring for
// ?since= replay. Extracted from run.go; the Service drives runs, the Hub
// only moves events.
package run

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

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
// for ?since= replay. Slow subscribers are evicted (they re-attach with
// since) so a stuck client never stalls the run.
type Hub struct {
	mu     sync.Mutex
	seq    int64
	buf    []Frame
	subs   map[string]chan Frame
	closed bool
	done   chan struct{}
	// Outcome is set by Close: "done" or "aborted".
	Outcome string
}

const hubBuffer = 500
const subBuffer = 256

// NewHub creates a broadcast hub (exported for tests and attach flows).
func NewHub() *Hub {
	return &Hub{subs: map[string]chan Frame{}, done: make(chan struct{})}
}

// Broadcast assigns the next sequence number and delivers to subscribers.
func (h *Hub) Broadcast(event loop.Event) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.seq++
	frame := Frame{Seq: h.seq, Event: event}
	h.buf = append(h.buf, frame)
	if len(h.buf) > hubBuffer {
		h.buf = h.buf[len(h.buf)-hubBuffer:]
	}
	for id, ch := range h.subs {
		select {
		case ch <- frame:
		default:
			delete(h.subs, id)
			close(ch)
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
	c := make(chan Frame, subBuffer)
	h.subs[id] = c
	if since != nil {
		for _, f := range h.buf {
			if f.Seq > *since {
				replay = append(replay, f)
			}
		}
	}
	return id, c, replay
}

// Unsubscribe removes a subscriber and drains its channel.
func (h *Hub) Unsubscribe(id string) {
	h.mu.Lock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
	h.mu.Unlock()
}

// Close terminates the hub with an outcome, waking settle waiters.
func (h *Hub) Close(outcome string) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.Outcome = outcome
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
	close(h.done)
	h.mu.Unlock()
}

// Done closes when the run settles.
func (h *Hub) Done() <-chan struct{} { return h.done }

func newSubID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "sub_" + hex.EncodeToString(b)
}
