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

// StripImagePayloads returns a tool result's content with every inline image
// part replaced by a short text placeholder naming the mime type. The base64
// payload is dropped entirely: it is only ever useful to the model on the turn
// that produced it, and keeping it would store megabytes per image in the
// session database and re-upload them on every later turn.
//
// A result with no image parts is returned unchanged, so text-only results keep
// their exact shape.
func StripImagePayloads(content any) any {
	switch v := content.(type) {
	case []map[string]any:
		out := make([]map[string]any, 0, len(v))
		changed := false
		for _, item := range v {
			if item["type"] == "image" {
				out = append(out, map[string]any{
					"type": "text",
					"text": imagePlaceholderText(item["mimeType"]),
				})
				changed = true
				continue
			}
			out = append(out, item)
		}
		if !changed {
			return content
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		changed := false
		for _, item := range v {
			if isImagePart(item) {
				mime := ""
				if m, ok := item.(map[string]any); ok {
					mime, _ = m["mimeType"].(string)
				}
				out = append(out, map[string]any{
					"type": "text",
					"text": imagePlaceholderText(mime),
				})
				changed = true
				continue
			}
			out = append(out, item)
		}
		if !changed {
			return content
		}
		return out
	default:
		return content
	}
}

// HasImageParts reports whether a tool result's content carries any inline
// image part. Callers use it to tell "nothing was stripped" from "the content
// changed shape", which interface equality cannot express for slice values.
func HasImageParts(content any) bool {
	switch v := content.(type) {
	case []map[string]any:
		for _, item := range v {
			if item["type"] == "image" {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if isImagePart(item) {
				return true
			}
		}
	}
	return false
}

// imagePlaceholderText is the stand-in left behind by a stripped image. It
// tells the model the image existed without pretending it can still see it.
func imagePlaceholderText(mime any) string {
	if s, ok := mime.(string); ok && s != "" {
		return "[image omitted: " + s + " image content was not retained]"
	}
	return "[image omitted: image content was not retained]"
}
