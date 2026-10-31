package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

const (
	waitDefault = 30 * time.Second
	// waitMax keeps one call well inside the kernel's own timeout (5 minutes by
	// default). A caller that needs longer waits again.
	waitMax      = 60 * time.Second
	waitPoll     = 200 * time.Millisecond
	waitMaxIDs   = 20
	replyMaxText = 20_000
)

// clampWait turns a requested timeout in milliseconds into a duration within
// [0, waitMax]; zero or absent means the default.
func clampWait(ms int, given bool) time.Duration {
	if !given || ms == 0 {
		return waitDefault
	}
	d := time.Duration(ms) * time.Millisecond
	if d > waitMax {
		return waitMax
	}
	return d
}

// waitIDs reads session_id (one) or session_ids (several), deduplicated, in the
// order given.
func waitIDs(args map[string]any) ([]string, error) {
	var ids []string
	if one, err := optionalString(args, "session_id"); err != nil {
		return nil, err
	} else if one != "" {
		ids = append(ids, one)
	}
	if raw, present := args["session_ids"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("session_ids must be an array of session ids")
		}
		for _, item := range list {
			id, ok := item.(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("session_ids must contain only non-empty strings, got %v", item)
			}
			ids = append(ids, id)
		}
	}
	seen := map[string]bool{}
	out := ids[:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("session_id or session_ids is required")
	}
	if len(out) > waitMaxIDs {
		return nil, fmt.Errorf("can wait on at most %d sessions at once, got %d", waitMaxIDs, len(out))
	}
	return out, nil
}

// sessionsWait blocks until the named sessions have finished their runs, or a
// bounded timeout passes. It is how an agent collects the result of work it
// started with sessions.create or sessions.prompt without polling.
//
// A session is finished when it has no active run. mode "all" (default) waits
// for every session; "any" returns as soon as one finishes. On timeout it
// returns what it has with done: false, and the caller can wait again.
func (r *Runtime) sessionsWait(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	ids, err := waitIDs(args)
	if err != nil {
		return nil, err
	}
	mode, err := optionalString(args, "mode")
	if err != nil {
		return nil, err
	}
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "any" {
		return nil, fmt.Errorf(`mode must be "all" or "any", got %q`, mode)
	}
	ms, hasMs, err := optionalInt(args, "timeout_ms")
	if err != nil {
		return nil, err
	}
	if ms < 0 {
		return nil, fmt.Errorf("timeout_ms must not be negative")
	}
	timeout := clampWait(ms, hasMs)

	for _, id := range ids {
		if id == r.sessionID {
			// It would wait for the run that is waiting, forever.
			return nil, fmt.Errorf("cannot wait on the session you are running in (%q)", id)
		}
		header, err := r.sessions.Header(id)
		if err != nil {
			return nil, err
		}
		if header == nil {
			return nil, fmt.Errorf("session %q not found", id)
		}
	}

	finished := func(id string) bool { return r.active == nil || !r.active(id) }
	satisfied := func() bool {
		if mode == "any" {
			for _, id := range ids {
				if finished(id) {
					return true
				}
			}
			return false
		}
		for _, id := range ids {
			if !finished(id) {
				return false
			}
		}
		return true
	}

	start := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(waitPoll)
	defer tick.Stop()
	timedOut := false
	for !satisfied() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			timedOut = true
		case <-tick.C:
		}
		if timedOut {
			break
		}
	}

	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		done := finished(id)
		entry := map[string]any{"session_id": id, "done": done, "status": r.waitStatus(id, done)}
		if done {
			if reply := r.lastReply(id); reply != "" {
				entry["last_reply"] = reply
			}
		}
		out = append(out, entry)
	}
	result := map[string]any{
		"done":      satisfied(),
		"timed_out": timedOut && !satisfied(),
		"waited_ms": time.Since(start).Milliseconds(),
		"sessions":  out,
	}
	if timedOut && !satisfied() {
		result["note"] = "still running; call sessions.wait again to keep waiting"
	}
	return marshalResult(result)
}

// waitStatus reports a session's status. A session with a live run is working
// whatever the stored header says (it is written a moment after the run starts).
func (r *Runtime) waitStatus(id string, done bool) string {
	if !done {
		return "working"
	}
	header, err := r.sessions.Header(id)
	if err != nil || header == nil {
		return "unknown"
	}
	if header.Status == "working" {
		// No active run but the header still says working: the run ended and
		// the header has not caught up.
		return "done"
	}
	return header.Status
}

// lastReply is the text of the session's most recent assistant message, cut at
// replyMaxText characters. Read the session for the full transcript.
func (r *Runtime) lastReply(id string) string {
	loaded, err := r.sessions.Load(id, 20, 0)
	if err != nil || loaded == nil {
		return ""
	}
	for i := len(loaded.Messages) - 1; i >= 0; i-- {
		var msg struct {
			Assistant *struct {
				Content []struct {
					Text *struct {
						Text string `json:"text"`
					} `json:"text"`
				} `json:"content"`
			} `json:"assistant"`
		}
		if json.Unmarshal(loaded.Messages[i], &msg) != nil || msg.Assistant == nil {
			continue
		}
		var parts []string
		for _, part := range msg.Assistant.Content {
			if part.Text != nil && strings.TrimSpace(part.Text.Text) != "" {
				parts = append(parts, part.Text.Text)
			}
		}
		if len(parts) == 0 {
			continue // a tool-call-only turn; keep looking for words
		}
		text := strings.Join(parts, "\n")
		if len(text) > replyMaxText {
			text = text[:replyMaxText] + fmt.Sprintf("\n… [reply cut: %d more characters; read the session for the rest]", len(text)-replyMaxText)
		}
		return text
	}
	return ""
}
