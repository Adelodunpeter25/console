// Tool-result images are in-flight only: the model sees the real image on the
// turn that produced it, but the base64 never reaches the session database and
// is never re-uploaded on a later turn.
package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// imageToolResult builds the content shape a tool returns when it carries an
// inline image: a text part plus a base64 image part.
func imageToolResult(mime, data string) []map[string]any {
	return []map[string]any{
		{"type": "text", "text": "Image read: /tmp/shot.png"},
		{"type": "image", "data": data, "mimeType": mime},
	}
}

// A stored tool result must carry no base64, only a short marker naming the
// image. This is the single boundary where image bytes enter storage.
func TestToolResultImageNotPersisted(t *testing.T) {
	big := strings.Repeat("A", 200_000)
	results := []tools.ToolResult{{
		ToolCallID: "c1",
		ToolName:   "read_file",
		Content:    imageToolResult("image/png", big),
	}}

	msg := loop.ToolResultMessage{Role: loop.RoleToolResult, Results: results}
	raw, err := loop.ToProtoBytes(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 4096 {
		t.Fatalf("stored tool result must not carry the payload: %d bytes", len(raw))
	}

	// Replaying the stored result yields a text-only marker, not an image
	// part — a later turn cannot re-upload what was never kept.
	decoded, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	replayed, ok := decoded.(loop.ToolResultMessage)
	if !ok {
		t.Fatalf("decoded type: %T", decoded)
	}
	content, ok := replayed.Results[0].Content.([]any)
	if !ok {
		t.Fatalf("replayed content type: %T", replayed.Results[0].Content)
	}
	for _, part := range content {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "image" {
			t.Fatalf("replayed result still carries an image part: %#v", m)
		}
	}
	marker, _ := content[1].(map[string]any)
	text, _ := marker["text"].(string)
	if !strings.Contains(text, "image/png") {
		t.Fatalf("stored result should name the image type: %q", text)
	}
	if strings.Contains(text, big[:1000]) {
		t.Fatal("stored tool result still contains base64 image data")
	}
}

// The in-memory history keeps the real image: the model must see the picture
// on the turn it acts on, even though storage drops it.
func TestToolResultImageSurvivesInMemory(t *testing.T) {
	data := "real-base64-payload"
	results := []tools.ToolResult{{
		ToolCallID: "c1",
		ToolName:   "read_file",
		Content:    imageToolResult("image/png", data),
	}}

	msg := loop.ToolResultMessage{Role: loop.RoleToolResult, Results: results}
	parts, ok := msg.Results[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("content type: %T", msg.Results[0].Content)
	}
	found := false
	for _, p := range parts {
		if p["type"] == "image" && p["data"] == data {
			found = true
		}
	}
	if !found {
		t.Fatal("in-memory history must keep the real image payload")
	}
}

// StripImagePayloads replaces images with a marker and leaves text-only
// results byte-identical.
func TestStripImagePayloads(t *testing.T) {
	// Text-only content is returned unchanged (same backing value).
	textOnly := []map[string]any{{"type": "text", "text": "hello"}}
	if got := loop.StripImagePayloads(textOnly); !sameContent(got, textOnly) {
		t.Fatalf("text-only content changed: %#v", got)
	}
	if loop.HasImageParts(textOnly) {
		t.Fatal("text-only content must not report image parts")
	}

	// An image becomes a text marker naming its mime type.
	stripped := loop.StripImagePayloads(imageToolResult("image/png", "payload"))
	parts, ok := stripped.([]map[string]any)
	if !ok {
		t.Fatalf("stripped type: %T", stripped)
	}
	if len(parts) != 2 {
		t.Fatalf("stripped parts: %#v", parts)
	}
	if parts[1]["type"] != "text" {
		t.Fatalf("image part not replaced by text: %#v", parts[1])
	}
	text, _ := parts[1]["text"].(string)
	if !strings.Contains(text, "image/png") {
		t.Fatalf("marker should name the image type: %q", text)
	}
	if strings.Contains(text, "payload") {
		t.Fatalf("marker must not carry payload data: %q", text)
	}
	if !loop.HasImageParts(imageToolResult("image/png", "payload")) {
		t.Fatal("image parts must be detected")
	}

	// The []any shape (decoded from stored JSON) is handled too.
	decoded := []any{
		map[string]any{"type": "text", "text": "hi"},
		map[string]any{"type": "image", "data": "payload", "mimeType": "image/webp"},
	}
	strippedAny := loop.StripImagePayloads(decoded)
	items, ok := strippedAny.([]any)
	if !ok {
		t.Fatalf("stripped type: %T", strippedAny)
	}
	marker, ok := items[1].(map[string]any)
	if !ok || marker["type"] != "text" {
		t.Fatalf("image part not replaced in []any shape: %#v", items[1])
	}
	if text, _ := marker["text"].(string); !strings.Contains(text, "image/webp") {
		t.Fatalf("marker should name the image type: %q", text)
	}
}

// sameContent compares two content values by their JSON form. Slices are not
// comparable in Go, so the test compares what the provider would see.
func sameContent(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// A provider turn that carries a tool result with an image must send the real
// image to the model, while the persisted copy stays stripped.
func TestImageDeliveredToModelButNotStored(t *testing.T) {
	data := "payload-for-the-model"
	results := []tools.ToolResult{{
		ToolCallID: "c1",
		ToolName:   "read_file",
		Content:    imageToolResult("image/png", data),
	}}

	// What the provider sees this turn: the in-memory result, unstripped.
	seen := results[0].Content
	parts, ok := seen.([]map[string]any)
	if !ok {
		t.Fatalf("content type: %T", seen)
	}
	delivered := false
	for _, p := range parts {
		if p["type"] == "image" && p["data"] == data {
			delivered = true
		}
	}
	if !delivered {
		t.Fatal("the model must receive the real image on the producing turn")
	}

	// What storage keeps: no payload.
	stored := loop.StripImagePayloads(seen)
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), data) {
		t.Fatal("stored form must not contain the payload")
	}
}

// The stripped marker must not look like a provider-level redaction, so the
// model can tell the difference between "no image support" and "not retained".
func TestStrippedMarkerDistinguishable(t *testing.T) {
	stripped := loop.StripImagePayloads(imageToolResult("image/png", "payload"))
	parts := stripped.([]map[string]any)
	text, _ := parts[1]["text"].(string)
	if strings.Contains(text, "provider does not support images") {
		t.Fatalf("stripped marker must not mimic the provider redaction: %q", text)
	}
	if !strings.Contains(text, "not retained") {
		t.Fatalf("marker should explain the image was dropped: %q", text)
	}
}

// The stripped marker must survive the provider adapters without being
// re-rendered as an image or dropped entirely.
func TestStrippedMarkerRendersAsText(t *testing.T) {
	stored := loop.StripImagePayloads(imageToolResult("image/png", "payload"))
	msg := loop.ToolResultMessage{
		Role:    loop.RoleToolResult,
		Results: []tools.ToolResult{{ToolCallID: "c1", Content: stored}},
	}
	raw, err := loop.ToProtoBytes(msg)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	replayed := decoded.(loop.ToolResultMessage)
	content, ok := replayed.Results[0].Content.([]any)
	if !ok {
		t.Fatalf("content type: %T", replayed.Results[0].Content)
	}
	if len(content) != 2 {
		t.Fatalf("parts: %#v", content)
	}
	marker, _ := content[1].(map[string]any)
	if marker["type"] != "text" {
		t.Fatalf("marker must be text: %#v", marker)
	}
	if text, _ := marker["text"].(string); !strings.Contains(text, "image omitted") {
		t.Fatalf("marker text: %q", text)
	}
}
