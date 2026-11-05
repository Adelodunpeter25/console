// Wire types for browser actions: the request the server sends and the
// result the desktop client answers with. The `browser` tool that drives
// them lives in internal/services/browseruse (the JS kernel); this file
// carries only the bridge both sides of the wire share.
package tools

import "context"

// BrowserActionRequest is dispatched to the connected desktop client.
type BrowserActionRequest struct {
	RequestID string `json:"requestId"`
	Action    string `json:"action"`
	URL       string `json:"url,omitempty"`
	Script    string `json:"script,omitempty"`
	Selector  string `json:"selector,omitempty"`
	TabID     string `json:"tabId,omitempty"`
	// wait_for conditions (all provided conditions must hold).
	URLContains string `json:"urlContains,omitempty"`
	Text        string `json:"text,omitempty"`
	TimeoutMs   int    `json:"timeoutMs,omitempty"`
	// Element refs from 'snapshot', used by click/type.
	Ref    string `json:"ref,omitempty"`
	Submit bool   `json:"submit,omitempty"`
	// NewTab forces navigate to open a fresh tab instead of reusing one
	// already on the URL. The kernel's browser.open uses it so the tab it
	// opens is unambiguously its own, and reset closes only those.
	NewTab bool `json:"newTab,omitempty"`
}

// BrowserActionResult is the result received from the desktop client.
type BrowserActionResult struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	// ImageBase64 is an optional PNG payload (screenshot action).
	ImageBase64 string `json:"imageBase64,omitempty"`
}

// BrowserHandler handles one browser action by delegating to the client.
type BrowserHandler func(ctx context.Context, req BrowserActionRequest) (BrowserActionResult, error)
