package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
)

const (
	waitDefault = 30 * time.Second
	// waitMax keeps one call well inside the kernel's own timeout (5 minutes by
	// default). A caller that needs longer waits again.
	waitMax = 60 * time.Second
	// waitBackgroundMax is both the default and the cap for a background wait,
	// which holds no turn open and so can afford to be long.
	waitBackgroundMax = 30 * time.Minute
	// backgroundSummaryMax mirrors the background manager's summary limit.
	backgroundSummaryMax = 2000
	waitPoll             = 200 * time.Millisecond
	waitMaxIDs           = 20
	replyMaxText         = 20_000
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
//
// With background: true it returns a ticket at once and the wait continues as
// background work, which wakes the calling session when it ends.
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
	inBackground := false
	if raw, present := args["background"]; present {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("background must be true or false, got %v", raw)
		}
		inBackground = b
	}

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

	wait := sessionWait{r: r, ids: ids, mode: mode}
	if inBackground && !wait.satisfied() {
		return r.startBackgroundWait(wait, clampBackgroundWait(ms, hasMs))
	}

	start := time.Now()
	timedOut, err := wait.poll(ctx, clampWait(ms, hasMs))
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"done":      wait.satisfied(),
		"timed_out": timedOut,
		"waited_ms": time.Since(start).Milliseconds(),
		"sessions":  wait.entries(replyMaxText),
	}
	if timedOut {
		result["note"] = "still running; call sessions.wait again to keep waiting"
	}
	return marshalResult(result)
}

// sessionWait is one wait over a fixed set of sessions. It reads only the
// active-run check and the session store, never the JS kernel, so a background
// wait outlives the kernel that started it.
type sessionWait struct {
	r    *Runtime
	ids  []string
	mode string
}

func (w sessionWait) finished(id string) bool { return w.r.active == nil || !w.r.active(id) }

func (w sessionWait) satisfied() bool {
	if w.mode == "any" {
		for _, id := range w.ids {
			if w.finished(id) {
				return true
			}
		}
		return false
	}
	for _, id := range w.ids {
		if !w.finished(id) {
			return false
		}
	}
	return true
}

// poll waits until the wait is satisfied or timeout passes, and reports
// whether it gave up on the timeout.
func (w sessionWait) poll(ctx context.Context, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(waitPoll)
	defer tick.Stop()
	for !w.satisfied() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return !w.satisfied(), nil
		case <-tick.C:
		}
	}
	return false, nil
}

// entries reports each session's state, with its last reply (cut at replyMax
// characters) once it has finished.
func (w sessionWait) entries(replyMax int) []map[string]any {
	out := make([]map[string]any, 0, len(w.ids))
	for _, id := range w.ids {
		done := w.finished(id)
		entry := map[string]any{"session_id": id, "done": done, "status": w.r.waitStatus(id, done)}
		if done {
			if reply := w.r.lastReply(id, replyMax); reply != "" {
				entry["last_reply"] = reply
			}
		}
		out = append(out, entry)
	}
	return out
}

// clampBackgroundWait is clampWait for a background wait, which defaults to
// and is capped at waitBackgroundMax.
func clampBackgroundWait(ms int, given bool) time.Duration {
	if !given || ms == 0 {
		return waitBackgroundMax
	}
	d := time.Duration(ms) * time.Millisecond
	if d > waitBackgroundMax {
		return waitBackgroundMax
	}
	return d
}

// startBackgroundWait registers the wait as background work owned by the
// calling session and returns its ticket. The work ends as completed when the
// wait is satisfied or timed_out at the deadline; both wake the caller.
func (r *Runtime) startBackgroundWait(wait sessionWait, timeout time.Duration) (*jskernel.Result, error) {
	var manager *background.Manager
	if r.work != nil {
		manager = r.work()
	}
	if manager == nil || r.sessionID == "" {
		return nil, fmt.Errorf("background waits are not available in this context; call sessions.wait without background")
	}
	work, err := manager.Register(background.Options{
		SessionID: r.sessionID,
		Kind:      background.KindSession,
		Metadata:  map[string]string{"session_ids": strings.Join(wait.ids, ","), "mode": wait.mode},
	})
	if err != nil {
		return nil, fmt.Errorf("could not start background wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.SetCancel(work.ID, cancel); err != nil {
		cancel()
		return nil, fmt.Errorf("could not start background wait: %v", err)
	}
	owner := r.sessionID
	go func() {
		defer cancel()
		timedOut, err := wait.poll(ctx, timeout)
		if err != nil {
			return // cancelled: the manager has already recorded it
		}
		status, failure := background.StatusCompleted, ""
		if timedOut {
			status = background.StatusTimedOut
			failure = fmt.Sprintf("gave up after %s with sessions still running; they were not stopped", timeout)
		}
		_, _ = manager.Complete(work.ID, owner, status, wait.summary(), failure, nil)
	}()
	return marshalResult(map[string]any{
		"background":  true,
		"work_id":     work.ID,
		"max_wait_ms": timeout.Milliseconds(),
		"sessions":    wait.entries(0),
		"note":        "waiting in the background; you will be notified with the result when the sessions finish, so end your turn or carry on with other work instead of waiting again",
	})
}

// summary renders the finished wait for the background-work callback: one
// status line per session, then the finished sessions' last replies sharing
// what is left of the manager's summary limit.
func (w sessionWait) summary() string {
	const hint = "Replies are cut to fit. For the full last replies call console.sessions.wait on the same sessions (it returns at once), or console.sessions.read."
	entries := w.entries(replyMaxText)
	var b strings.Builder
	replies := 0
	for _, entry := range entries {
		fmt.Fprintf(&b, "%s: %s\n", entry["session_id"], entry["status"])
		if _, ok := entry["last_reply"]; ok {
			replies++
		}
	}
	if replies == 0 {
		return strings.TrimSpace(b.String())
	}
	// Per reply: its share of the budget, less the heading and cut marker.
	share := (backgroundSummaryMax-b.Len()-len(hint))/replies - 80
	for _, entry := range entries {
		reply, ok := entry["last_reply"].(string)
		if !ok {
			continue
		}
		if share <= 0 {
			break
		}
		if len(reply) > share {
			reply = strings.ToValidUTF8(reply[:share], "") + " … [cut]"
		}
		fmt.Fprintf(&b, "\nLast reply from %s:\n%s\n", entry["session_id"], reply)
	}
	b.WriteString("\n" + hint)
	return b.String()
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
// max characters (zero skips the read). Read the session for the full transcript.
func (r *Runtime) lastReply(id string, max int) string {
	if max <= 0 {
		return ""
	}
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
		if len(text) > max {
			text = text[:max] + fmt.Sprintf("\n… [reply cut: %d more characters; read the session for the rest]", len(text)-max)
		}
		return text
	}
	return ""
}

// sessionsSearch finds messages that contain every word of a query. It calls the
// same session.Service.Search a future in-chat Cmd+F will call, so there is one
// definition of a match.
//
// By default it searches the current session's project (or, with no project, its
// directory) and leaves out the session the agent is running in: that one is
// already in context, and finding the agent's own words again is noise.
func (r *Runtime) sessionsSearch(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	query, err := stringArg(args, "query")
	if err != nil {
		return nil, err
	}
	scope := session.SearchScope{Exclude: []string{r.sessionID}}
	sessionID, err := optionalString(args, "session_id")
	if err != nil {
		return nil, err
	}
	projectID, err := optionalString(args, "project_id")
	if err != nil {
		return nil, err
	}
	cwd, err := optionalString(args, "cwd")
	if err != nil {
		return nil, err
	}
	allProjects, _ := args["all_projects"].(bool)
	if raw, present := args["all_projects"]; present {
		if _, ok := raw.(bool); !ok {
			return nil, fmt.Errorf("all_projects must be true or false, got %v", raw)
		}
	}
	if raw, present := args["include_deleted"]; present {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("include_deleted must be true or false, got %v", raw)
		}
		scope.IncludeDeleted = b
	}

	scoped := 0
	for _, set := range []bool{sessionID != "", projectID != "", cwd != "", allProjects} {
		if set {
			scoped++
		}
	}
	if scoped > 1 {
		return nil, fmt.Errorf("choose one scope: session_id, project_id, cwd or all_projects")
	}
	switch {
	case sessionID != "":
		// Searching one named session is deliberate; do not exclude it, except
		// that the agent's own session is still not worth searching.
		scope.SessionID = sessionID
		if sessionID == r.sessionID {
			return nil, fmt.Errorf("that is the session you are running in; its messages are already in your context")
		}
	case projectID != "":
		scope.ProjectID = projectID
	case cwd != "":
		scope.Cwd = cwd
	case allProjects:
		// every session
	default:
		// The current project, or the current directory when there is none.
		if r.projectID != "" {
			scope.ProjectID = r.projectID
		} else if r.cwd != "" {
			scope.Cwd = r.cwd
		} else {
			return nil, fmt.Errorf("this session has no project or directory to search; pass project_id, cwd, session_id or all_projects: true")
		}
	}

	var opts session.SearchOptions
	if raw, present := args["roles"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf(`roles must be an array such as ["user","assistant"]`)
		}
		for _, item := range list {
			role, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("roles must contain only strings, got %v", item)
			}
			opts.Roles = append(opts.Roles, role)
		}
	}
	if n, ok, err := optionalInt(args, "limit"); err != nil {
		return nil, err
	} else if ok {
		if n <= 0 {
			return nil, fmt.Errorf("limit must be positive")
		}
		opts.Limit = n
	}
	if n, ok, err := optionalInt(args, "per_session"); err != nil {
		return nil, err
	} else if ok {
		if n <= 0 {
			return nil, fmt.Errorf("per_session must be positive")
		}
		opts.PerSession = n
	}

	result, err := r.sessions.Search(scope, query, opts)
	if err != nil {
		return nil, err
	}
	return marshalResult(result)
}
