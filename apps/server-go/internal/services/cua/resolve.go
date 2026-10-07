// Element resolution and launch waiting for the computer-use runtime.
//
// Row tokens (snapshot_id:row) renumber between snapshots, but the driver's
// `id=` attributes are stable per element. Accepting element_id lets scripts
// address what they mean instead of where it happened to be listed: the
// runtime snapshots, matches the id, and acts with a fresh token, all inside
// one call. A miss fails loudly with the ids that ARE present, never silently
// with the wrong element.
//
// launch_app gets the same treatment for its own race: a fresh launch
// reports window_ready=false because the window does not exist yet, so the
// runtime polls list_windows until it appears (or a timeout) rather than
// making every script hand-roll the wait.
package cua

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// elementMethods are the driver methods that address an element by token and
// therefore accept element_id. Anything else carrying element_id is a script
// bug, reported as such rather than forwarded to a deny_unknown_fields
// rejection from the driver.
var elementMethods = map[string]bool{
	"click":        true,
	"double_click": true,
	"right_click":  true,
	"type_text":    true,
	"press_key":    true,
	"hotkey":       true,
	"set_value":    true,
	"scroll":       true,
}

// launchPollInterval paces the launch wait. launchWaitTimeoutMs bounds it;
// hou
const (
	launchPollInterval       = 500 * time.Millisecond
	launchWaitTimeoutMs      = 15000
	resolveSnapshotTimeoutMs = 10000
)

// SnapshotRow is one addressable row of a window snapshot: its row number
// plus the stable id= attribute. Rows without an id are not addressable and
// never appear here.
type SnapshotRow struct {
	Row int
	ID  string
}

// snapshotRowRe matches one addressable tree row. The id= match is anchored
// to the trailing metadata block so a description that merely mentions
// "[id=...]" mid-line can never resolve: only the driver's own attribute
// block counts.
//
//   - [5] AXButton (7) [id=Seven actions=[press]]
var snapshotRowRe = regexp.MustCompile(`^\s*- \[(\d+)\][^\n]*\[id=([^\s\]]+)(?:\s+.*)?\]\s*$`)

// ParseSnapshotRowsForTest exposes row parsing so adversarial shapes are
// pinned without a driver: descriptions mentioning ids, rows without index,
// rows without id block.
func ParseSnapshotRowsForTest(markdown string) []SnapshotRow {
	return parseSnapshotRows(markdown)
}

func parseSnapshotRows(markdown string) []SnapshotRow {
	var out []SnapshotRow
	for _, line := range strings.Split(markdown, "\n") {
		m := snapshotRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var row SnapshotRow
		fmt.Sscanf(m[1], "%d", &row.Row)
		row.ID = m[2]
		out = append(out, row)
	}
	return out
}

// resolveElementID snapshots the window and maps a stable id= attribute to a
// fresh element token. The snapshot is taken inside this call so the token
// cannot go stale between observing and acting.
func (r *JSRuntime) resolveElementID(ctx context.Context, pid, windowID int64, elementID string) (token string, err error) {
	result, err := r.caller.Call(ctx, "get_window_state", map[string]any{
		"pid": pid, "window_id": windowID, "timeout_ms": resolveSnapshotTimeoutMs,
	}, r.cancel)
	if err != nil {
		return "", fmt.Errorf("could not snapshot the window to resolve %q: %w", elementID, err)
	}
	var snapshot struct {
		SnapshotID string `json:"snapshot_id"`
		Tree       string `json:"tree_markdown"`
		Truncated  bool   `json:"truncated"`
	}
	if len(result.StructuredContent) > 0 {
		_ = json.Unmarshal(result.StructuredContent, &snapshot)
	}
	if snapshot.SnapshotID == "" {
		return "", fmt.Errorf("could not resolve %q: the snapshot carried no id", elementID)
	}
	rows := parseSnapshotRows(snapshot.Tree)
	for _, row := range rows {
		if row.ID == elementID {
			return fmt.Sprintf("%s:%d", snapshot.SnapshotID, row.Row), nil
		}
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	truncated := ""
	if snapshot.Truncated {
		truncated = " (tree truncated: the id may exist past the cut)"
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no element with id %q in snapshot %s: the window exposes no addressable elements%s", elementID, snapshot.SnapshotID, truncated)
	}
	listed := strings.Join(ids, ", ")
	if len(listed) > 500 {
		listed = listed[:500] + "…"
	}
	return "", fmt.Errorf("no element with id %q in snapshot %s%s; present ids: %s", elementID, snapshot.SnapshotID, truncated, listed)
}

// toInt64 reads an integer out of script-decoded arguments, which arrive as
// float64 or int64 depending on the value. Window ids exceed 32 bits, so the
// conversion must not narrow.
func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

// resolveElementArg implements element_id for one call: validates, resolves,
// and rewrites args in place (element_id is ours; the driver's wire inputs
// deny unknown fields, so it must never be forwarded).
func (r *JSRuntime) resolveElementArg(ctx context.Context, name string, args map[string]any) error {
	rawID, ok := args["element_id"].(string)
	if !ok || strings.TrimSpace(rawID) == "" {
		return fmt.Errorf("element_id must be a non-empty string")
	}
	elementID := strings.TrimSpace(rawID)
	if !elementMethods[name] {
		return fmt.Errorf("element_id is only meaningful for click, double_click, right_click, type_text, press_key, hotkey, set_value and scroll — not %s", name)
	}
	if token, present := args["element_token"]; present && token != "" && token != nil {
		// An explicit token is fresher knowledge than an id: use it.
		delete(args, "element_id")
		return nil
	}
	pid, ok := toInt64(args["pid"])
	if !ok {
		return fmt.Errorf("element_id needs a numeric pid to snapshot")
	}
	windowID, ok := toInt64(args["window_id"])
	if !ok {
		return fmt.Errorf("element_id needs a numeric window_id to snapshot")
	}
	token, err := r.resolveElementID(ctx, pid, windowID, elementID)
	if err != nil {
		return err
	}
	delete(args, "element_id")
	args["element_token"] = token
	return nil
}

// waitLaunchWindow polls list_windows after a launch that reported no
// windows, and returns a note for the result. Empty string means nothing to
// add: windows were already there, the launch failed, or there is no pid to
// poll. waitTimeoutMs of zero selects the default.
func (r *JSRuntime) waitLaunchWindow(ctx context.Context, args map[string]any, result *ToolResult, waitTimeoutMs int) string {
	if result.IsError {
		return ""
	}
	var launched struct {
		PID     any   `json:"pid"`
		Windows []any `json:"windows"`
	}
	pid := int64(0)
	if len(result.StructuredContent) > 0 && json.Unmarshal(result.StructuredContent, &launched) == nil {
		pid, _ = toInt64(launched.PID)
	}
	if len(launched.Windows) > 0 || pid == 0 {
		return ""
	}
	if waitTimeoutMs <= 0 {
		waitTimeoutMs = launchWaitTimeoutMs
	}
	deadline := time.Now().Add(time.Duration(waitTimeoutMs) * time.Millisecond)
	start := time.Now()
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ""
		}
		state, err := r.caller.Call(ctx, "list_windows", map[string]any{"pid": pid}, r.cancel)
		if err == nil {
			var payload struct {
				Windows []struct {
					WindowID any    `json:"window_id"`
					Title    string `json:"title"`
				} `json:"windows"`
			}
			if len(state.StructuredContent) > 0 && json.Unmarshal(state.StructuredContent, &payload) == nil && len(payload.Windows) > 0 {
				id, _ := toInt64(payload.Windows[0].WindowID)
				title := payload.Windows[0].Title
				if title == "" {
					title = "untitled"
				}
				return fmt.Sprintf("Window %d (%q) appeared ~%dms after launch; snapshot it with get_window_state before acting.", id, title, time.Since(start).Milliseconds())
			}
		}
		time.Sleep(launchPollInterval)
	}
	return fmt.Sprintf("No window appeared on pid %d after %dms; it may still be launching or on another Space — check list_windows, then bring_to_front.", pid, waitTimeoutMs)
}
