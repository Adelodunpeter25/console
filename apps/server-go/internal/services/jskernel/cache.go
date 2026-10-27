package jskernel

import (
	"strings"
	"sync"
)

// Cache keeps one runtime per key, evicting the least recently used when the
// capacity is exceeded. Eviction only drops the map entry: a caller still
// holding a runtime keeps working, and the next Get for that key builds a
// fresh one. That matches a kernel whose variables are a convenience, never
// durable state.
//
// Capacity is in entries, so a caller that keeps two kernels per session
// (read-only and exec) should budget twice its session count.
type Cache[T any] struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]T
	order    []string
}

// NewCache returns a cache holding at most capacity entries. A non-positive
// capacity selects the default of 16.
func NewCache[T any](capacity int) *Cache[T] {
	if capacity <= 0 {
		capacity = 16
	}
	return &Cache[T]{capacity: capacity, entries: map[string]T{}}
}

// Get returns the entry for key, building it on first use. Build errors are
// not cached, so a transient failure can be retried.
func (c *Cache[T]) Get(key string, build func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		c.touchLocked(key)
		return entry, nil
	}
	entry, err := build()
	if err != nil {
		var zero T
		return zero, err
	}
	c.entries[key] = entry
	c.order = append(c.order, key)
	for len(c.order) > c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	return entry, nil
}

// Drop forgets one key. Unknown keys are ignored.
func (c *Cache[T]) Drop(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	for i, id := range c.order {
		if id == key {
			c.order = append(c.order[:i:i], c.order[i+1:]...)
			return
		}
	}
}

// DropPrefix forgets every key with the prefix, so a caller keyed by
// session:mode can reset one session's kernels in one call.
func (c *Cache[T]) DropPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.order[:0]
	for _, key := range c.order {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
			continue
		}
		kept = append(kept, key)
	}
	c.order = kept
}

// Len reports how many entries are live, for status surfaces and tests.
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *Cache[T]) touchLocked(key string) {
	for i, id := range c.order {
		if id == key {
			c.order = append(append(c.order[:i:i], c.order[i+1:]...), key)
			return
		}
	}
}
