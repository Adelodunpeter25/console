// Shared tool-result content inspection. Token estimators and compaction
// summaries must count a result's text without its inline base64 image
// payloads, and add a flat per-image allowance instead.
package loop

// SplitImageParts returns a tool result's content with every inline image part
// removed, plus the number of parts removed. Image parts are the
// {"type":"image","data":<base64>,"mimeType":<mime>} shape produced by the
// browser, MCP, CUA, and read_file tools. Both slice shapes are recognized:
// []map[string]any as tools return them in-memory, and []any as they decode
// back from stored JSON. A plain string or any other shape is returned
// unchanged with a zero count, so callers that only need character counts keep
// counting text exactly as before.
func SplitImageParts(content any) (any, int) {
	switch v := content.(type) {
	case []map[string]any:
		images := 0
		for _, item := range v {
			if item["type"] == "image" {
				images++
			}
		}
		if images == 0 {
			return content, 0
		}
		out := make([]map[string]any, 0, len(v)-images)
		for _, item := range v {
			if item["type"] != "image" {
				out = append(out, item)
			}
		}
		return out, images
	case []any:
		images := 0
		for _, item := range v {
			if isImagePart(item) {
				images++
			}
		}
		if images == 0 {
			return content, 0
		}
		out := make([]any, 0, len(v)-images)
		for _, item := range v {
			if !isImagePart(item) {
				out = append(out, item)
			}
		}
		return out, images
	default:
		return content, 0
	}
}

// isImagePart reports whether one tool-result content item is an inline image.
func isImagePart(part any) bool {
	m, ok := part.(map[string]any)
	return ok && m["type"] == "image"
}
