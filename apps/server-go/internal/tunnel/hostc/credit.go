package hostc

import "sync"

// credit is a flow-control window. A sender may have at most the granted
// bytes in flight; the receiver grants them back as it consumes them.
type credit struct {
	mu     sync.Mutex
	cond   *sync.Cond
	amount int
	closed bool
}

// newCredit returns a window with an initial allowance.
func newCredit(initial int) *credit {
	c := &credit{amount: initial}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// take blocks until n bytes are available. It reports false when the window
// is closed before that happens.
func (c *credit) take(n int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.amount < n && !c.closed {
		c.cond.Wait()
	}
	if c.closed {
		return false
	}
	c.amount -= n
	return true
}

// grant gives the peer more room.
func (c *credit) grant(n int) {
	c.mu.Lock()
	c.amount += n
	c.mu.Unlock()
	c.cond.Broadcast()
}

// close releases everyone waiting on the window.
func (c *credit) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cond.Broadcast()
}

// queue is an unbounded FIFO.
//
// The tunnel's request body and its WebSocket messages are queued without
// bound on purpose: the local server may stop reading them, and blocking the
// tunnel's read pump would stall every other stream sharing the connection.
type queue[T any] struct {
	mu     sync.Mutex
	items  []T
	signal chan struct{}
	closed bool
}

func newQueue[T any]() *queue[T] {
	return &queue[T]{signal: make(chan struct{}, 1)}
}

// push appends an item to the queue.
func (q *queue[T]) push(item T) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, item)
	q.mu.Unlock()
	q.wake()
}

// pop returns the next item, blocking until one arrives. It reports false
// once the queue is closed and drained.
func (q *queue[T]) pop() (T, bool) {
	var zero T
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			item := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return item, true
		}
		closed := q.closed
		q.mu.Unlock()
		if closed {
			return zero, false
		}
		<-q.signal
	}
}

// close marks the queue finished; pop drains what is left and then stops.
func (q *queue[T]) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.wake()
}

func (q *queue[T]) wake() {
	select {
	case q.signal <- struct{}{}:
	default:
	}
}
