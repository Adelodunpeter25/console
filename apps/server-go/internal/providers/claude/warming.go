package claude

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// Cache warming: Anthropic drops a cached prefix after its lifetime (one hour
// for long retention), so a conversation left idle for longer pays for a full
// cache write on its next turn. Shortly before the entry would expire, the
// last request is replayed unchanged; reading the cached prefix refreshes the
// lifetime. The replay is identical to the real request (same thinking
// budget and max_tokens, which Anthropic keys the cache on), and the stream is
// cancelled at message_start, where the cache usage is already reported, so
// almost no output is generated. Setting CONSOLE_CACHE_WARMING=off disables it.
const (
	warmingEnv          = "CONSOLE_CACHE_WARMING"
	warmMinPromptTokens = 20_000
	warmMaxTracked      = 32
	warmTimeout         = 2 * time.Minute
)

var (
	// warmDelay is 90% of the one-hour lifetime, leaving margin for a late timer.
	warmDelay = 54 * time.Minute
	// warmMaxIdle bounds how long an idle conversation keeps being warmed from
	// its last real request: each warm costs a cache read of the whole prompt.
	warmMaxIdle = 2 * time.Hour
)

var errWarmed = errors.New("cache warmed")

type warmEntry struct {
	gen      int
	timer    *time.Timer
	lastReal time.Time
}

var (
	warmMu      sync.Mutex
	warmEntries = map[string]*warmEntry{}
)

func warmingEnabled() bool { return os.Getenv(warmingEnv) != "off" }

// SetWarmTiming overrides the warming delays and returns a restore function
// (tests).
func SetWarmTiming(delay, maxIdle time.Duration) func() {
	warmMu.Lock()
	prevDelay, prevIdle := warmDelay, warmMaxIdle
	warmDelay, warmMaxIdle = delay, maxIdle
	warmMu.Unlock()
	return func() {
		warmMu.Lock()
		warmDelay, warmMaxIdle = prevDelay, prevIdle
		warmMu.Unlock()
	}
}

// StopAllWarming cancels every pending warm (tests, shutdown).
func StopAllWarming() {
	warmMu.Lock()
	defer warmMu.Unlock()
	for id, e := range warmEntries {
		e.gen++
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(warmEntries, id)
	}
}

// cancelWarm drops a conversation's pending warm: a real request is about to
// refresh the cache itself.
func cancelWarm(conversationID string) {
	if conversationID == "" {
		return
	}
	warmMu.Lock()
	defer warmMu.Unlock()
	if e, ok := warmEntries[conversationID]; ok {
		e.gen++
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(warmEntries, conversationID)
	}
}

// scheduleWarm arms a warm for a finished main-conversation turn. Only long
// retention is warmed (subagents and helper calls are not worth it), and only
// prompts large enough that a rewrite costs real money.
func (p *Provider) scheduleWarm(req loop.TurnRequest, body map[string]any, usage *loop.TurnUsage) {
	if !warmingEnabled() || req.ConversationID == "" || req.CacheRetention != loop.CacheLong {
		return
	}
	if usage == nil || usage.PromptTokens() < warmMinPromptTokens {
		return
	}
	warmMu.Lock()
	defer warmMu.Unlock()
	e, ok := warmEntries[req.ConversationID]
	if !ok {
		if len(warmEntries) >= warmMaxTracked {
			evictOldestWarmLocked()
		}
		e = &warmEntry{}
		warmEntries[req.ConversationID] = e
	}
	e.gen++
	if e.timer != nil {
		e.timer.Stop()
	}
	e.lastReal = time.Now()
	gen := e.gen
	e.timer = time.AfterFunc(warmDelay, func() { p.fireWarm(req.ConversationID, req.AccountID, gen, body) })
}

func evictOldestWarmLocked() {
	oldestID, oldest := "", time.Now()
	for id, e := range warmEntries {
		if e.lastReal.Before(oldest) {
			oldestID, oldest = id, e.lastReal
		}
	}
	if e, ok := warmEntries[oldestID]; ok {
		e.gen++
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(warmEntries, oldestID)
	}
}

func (p *Provider) fireWarm(conversationID, accountID string, gen int, body map[string]any) {
	warmMu.Lock()
	e, ok := warmEntries[conversationID]
	current := ok && e.gen == gen
	warmMu.Unlock()
	if !current {
		return
	}

	usage, err := p.replay(conversationID, accountID, body)
	if err != nil {
		slog.Info("cache warm failed", "conversation", conversationID, "error", err.Error())
		cancelWarmIfCurrent(conversationID, gen)
		return
	}
	slog.Info("cache warmed", "conversation", conversationID,
		"read", usage.CacheRead, "write", usage.CacheWrite, "input", usage.Input)

	warmMu.Lock()
	defer warmMu.Unlock()
	e, ok = warmEntries[conversationID]
	if !ok || e.gen != gen {
		return
	}
	if time.Since(e.lastReal)+warmDelay > warmMaxIdle {
		delete(warmEntries, conversationID)
		return
	}
	e.timer = time.AfterFunc(warmDelay, func() { p.fireWarm(conversationID, accountID, gen, body) })
}

func cancelWarmIfCurrent(conversationID string, gen int) {
	warmMu.Lock()
	defer warmMu.Unlock()
	if e, ok := warmEntries[conversationID]; ok && e.gen == gen {
		delete(warmEntries, conversationID)
	}
}

// replay sends the stored request and returns the usage reported at
// message_start, cancelling the stream right after.
func (p *Provider) replay(conversationID, accountID string, body map[string]any) (*loop.TurnUsage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), warmTimeout)
	defer cancel()
	cred, err := p.loadCredential(accountID)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, MessagesURL(p.baseURL()), cred.AccessToken, conversationID, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var usage *loop.TurnUsage
	err = parseAnthropicSSE(ctx, resp.Body, func(f sseFrame) error {
		if f.event != "message_start" {
			return nil
		}
		if message, ok := f.data["message"].(map[string]any); ok {
			if u, ok := message["usage"].(map[string]any); ok {
				usage = NormalizeUsage(u, nil, loop.CacheLong)
			}
		}
		return errWarmed
	})
	if usage == nil {
		if err != nil && !errors.Is(err, errWarmed) {
			return nil, err
		}
		return nil, errors.New("no usage in warm response")
	}
	return usage, nil
}
