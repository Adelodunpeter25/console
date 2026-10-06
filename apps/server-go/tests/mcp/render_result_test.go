// renderResult: MCP image content must survive as a native image part so the
// provider converters can build real image blocks (Claude), instead of the old
// "[image ... omitted]" text. Text, structured content, caps and ordering are
// covered too, because the same function feeds every MCP tool result.
package tests

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

type stubCaller struct {
	res *sdk.CallToolResult
}

func (s stubCaller) Call(_ context.Context, _, _ string, _ map[string]any) (*sdk.CallToolResult, error) {
	return s.res, nil
}

// call renders one MCP result through a real adapter tool, so the test covers
// the same path the agent takes.
func call(t *testing.T, res *sdk.CallToolResult) []map[string]any {
	t.Helper()
	tool := mcp.NewAdapter(stubCaller{res: res}, mcp.ServerConfig{ID: "cua"},
		mcp.RemoteTool{Name: "get_window_state"})
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	env, ok := out.(tools.Envelope)
	if !ok {
		t.Fatalf("got %T, want tools.Envelope", out)
	}
	parts, ok := env.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content is %T, want []map[string]any", env.Content)
	}
	return parts
}

func findPart(parts []map[string]any, kind string) map[string]any {
	for _, p := range parts {
		if p["type"] == kind {
			return p
		}
	}
	return nil
}

func TestRenderResultPassesImageThroughAsNativePart(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	parts := call(t, &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.TextContent{Text: "window 42"},
		&sdk.ImageContent{MIMEType: "image/png", Data: png},
	}})

	img := findPart(parts, "image")
	if img == nil {
		t.Fatalf("no image part in %v", parts)
	}
	if img["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v", img["mimeType"])
	}
	want := base64.StdEncoding.EncodeToString(png)
	if img["data"] != want {
		t.Errorf("data = %v want %v", img["data"], want)
	}
	if txt, _ := findPart(parts, "text")["text"].(string); !strings.Contains(txt, "window 42") {
		t.Errorf("text part lost its content: %v", txt)
	}
}

func TestRenderResultNoLongerMentionsOmittedImage(t *testing.T) {
	parts := call(t, &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.ImageContent{MIMEType: "image/png", Data: []byte{1, 2}},
	}})
	for _, p := range parts {
		if txt, _ := p["text"].(string); strings.Contains(txt, "omitted]") {
			t.Fatalf("image was still dropped: %q", txt)
		}
	}
}

func TestRenderResultImageOnlyStillReturnsImagePart(t *testing.T) {
	parts := call(t, &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.ImageContent{MIMEType: "image/jpeg", Data: []byte{9}},
	}})
	if findPart(parts, "image") == nil {
		t.Fatalf("image-only result lost its image: %v", parts)
	}
	// No text at all, so no "(no output)" filler competing for the context.
	if findPart(parts, "text") != nil {
		t.Fatalf("unexpected text part in image-only result: %v", parts)
	}
}

func TestRenderResultCapsImagesAndSaysHowManyWereDropped(t *testing.T) {
	var content []sdk.Content
	for i := 0; i < 9; i++ {
		content = append(content, &sdk.ImageContent{MIMEType: "image/png", Data: []byte{byte(i)}})
	}
	parts := call(t, &sdk.CallToolResult{Content: content})

	images := 0
	for _, p := range parts {
		if p["type"] == "image" {
			images++
		}
	}
	if images != 4 {
		t.Fatalf("got %d image parts, want the cap of 4", images)
	}
	txt, _ := findPart(parts, "text")["text"].(string)
	if !strings.Contains(txt, "5 more image(s) omitted") {
		t.Errorf("drop count not reported: %q", txt)
	}
}

func TestRenderResultStructuredContentUsedWhenNoParts(t *testing.T) {
	parts := call(t, &sdk.CallToolResult{StructuredContent: map[string]any{"window_id": 7}})
	txt, _ := findPart(parts, "text")["text"].(string)
	if !strings.Contains(txt, "window_id") {
		t.Errorf("structured content not surfaced: %q", txt)
	}
}

func TestRenderResultEmptyResultStillReportsSomething(t *testing.T) {
	parts := call(t, &sdk.CallToolResult{})
	txt, _ := findPart(parts, "text")["text"].(string)
	if txt != "(no output)" {
		t.Errorf("got %q, want %q", txt, "(no output)")
	}
}

func TestRenderResultAudioStaysATextNote(t *testing.T) {
	// The converters accept no audio block, so it must not become a fake part.
	parts := call(t, &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.AudioContent{MIMEType: "audio/wav", Data: []byte{1}},
	}})
	for _, p := range parts {
		if p["type"] != "text" && p["type"] != "image" {
			t.Fatalf("unexpected part type %v", p["type"])
		}
	}
	txt, _ := findPart(parts, "text")["text"].(string)
	if !strings.Contains(txt, "audio content omitted") {
		t.Errorf("audio note missing: %q", txt)
	}
}
