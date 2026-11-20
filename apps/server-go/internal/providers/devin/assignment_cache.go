package devin

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// A router model ("adaptive") picks a concrete model per request, and each
// model has its own prompt cache, so re-routing every turn splits the cache.
// The assignment is reused for a conversation until its JWT is about to
// expire; a JWT without a readable expiry is reused for fallbackAssignmentTTL.
const (
	assignmentExpiryMargin = time.Minute
	fallbackAssignmentTTL  = 10 * time.Minute
)

type pinnedAssignment struct {
	assignment *modelAssignment
	routerUID  string
	expires    time.Time
}

type assignmentCache struct {
	mu      sync.Mutex
	entries map[string]pinnedAssignment
}

func (c *assignmentCache) get(conversationID, routerUID string, now time.Time) (*modelAssignment, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[conversationID]
	if !ok || entry.routerUID != routerUID || !now.Before(entry.expires) {
		return nil, false
	}
	return entry.assignment, true
}

func (c *assignmentCache) put(conversationID, routerUID string, a *modelAssignment, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]pinnedAssignment{}
	}
	expires := now.Add(fallbackAssignmentTTL)
	if exp, ok := jwtExpiry(a.jwt); ok {
		expires = exp.Add(-assignmentExpiryMargin)
	}
	c.entries[conversationID] = pinnedAssignment{assignment: a, routerUID: routerUID, expires: expires}
}

func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// PinnedAssignments exposes the per-conversation router pin to tests.
type PinnedAssignments struct{ c assignmentCache }

// Get returns the pinned model uid for a conversation, if still valid.
func (p *PinnedAssignments) Get(conversationID, routerUID string, now time.Time) (string, bool) {
	a, ok := p.c.get(conversationID, routerUID, now)
	if !ok {
		return "", false
	}
	return a.modelUID, true
}

// Put pins a model uid (with its assignment JWT) for a conversation.
func (p *PinnedAssignments) Put(conversationID, routerUID, modelUID, jwt string, now time.Time) {
	p.c.put(conversationID, routerUID, &modelAssignment{jwt: jwt, modelUID: modelUID}, now)
}
