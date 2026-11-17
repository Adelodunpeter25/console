// Devin provider coverage: Connect stream decoding, prompt conversion, and
// model normalization against mock HTTP servers and pure-function checks.
package tests

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/devin"
)

func devinTestCredential() (devin.ParsedCredential, error) {
	return devin.ParsedCredential{Token: "session-token", Email: "dev@example.com"}, nil
}

// connectFrame wraps one message in a Connect stream envelope.
func connectFrame(t *testing.T, flag byte, payload []byte) []byte {
	t.Helper()
	frame := make([]byte, 5+len(payload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

// devinMockServer answers GetUserJwt (unary proto) and GetChatMessage
// (Connect stream of the given responses plus an end-stream trailer).
func devinMockServer(t *testing.T, chatResponses []*exav1.GetChatMessageResponse, trailer string, onChat func(*exav1.GetChatMessageRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exa.auth_pb.AuthService/GetUserJwt":
			raw, _ := io.ReadAll(r.Body)
			req := &exav1.GetUserJwtRequest{}
			if err := proto.Unmarshal(raw, req); err != nil {
				t.Errorf("GetUserJwt request decode: %v", err)
			}
			if !strings.HasPrefix(req.GetMetadata().GetApiKey(), "devin-session-token$") {
				t.Errorf("apiKey missing session prefix: %q", req.GetMetadata().GetApiKey())
			}
			resp, _ := proto.Marshal(&exav1.GetUserJwtResponse{UserJwt: "user-jwt-1"})
			w.Header().Set("content-type", "application/proto")
			_, _ = w.Write(resp)
		case "/exa.api_server_pb.ApiServerService/GetChatMessage":
			body, _ := io.ReadAll(r.Body)
			if len(body) < 5 || body[0]&0x01 == 0 {
				t.Errorf("chat request frame not flagged compressed")
			}
			length := int(binary.BigEndian.Uint32(body[1:5]))
			if len(body) < 5+length {
				t.Fatalf("chat frame truncated: %d < %d", len(body), 5+length)
			}
			zr, err := gzip.NewReader(bytes.NewReader(body[5 : 5+length]))
			if err != nil {
				t.Fatalf("chat frame gunzip: %v", err)
			}
			raw, _ := io.ReadAll(zr)
			chatReq := &exav1.GetChatMessageRequest{}
			if err := proto.Unmarshal(raw, chatReq); err != nil {
				t.Fatalf("chat request decode: %v", err)
			}
			if onChat != nil {
				onChat(chatReq)
			}
			var out bytes.Buffer
			for _, resp := range chatResponses {
				payload, _ := proto.Marshal(resp)
				out.Write(connectFrame(t, 0, payload))
			}
			out.Write(connectFrame(t, 0x02, []byte(trailer)))
			w.Header().Set("content-type", "application/connect+proto")
			_, _ = w.Write(out.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestDevinProviderStreamsTurn(t *testing.T) {
	var gotReq *exav1.GetChatMessageRequest
	srv := devinMockServer(t, []*exav1.GetChatMessageResponse{
		{DeltaThinking: "thinking...", DeltaSignature: "sig-1"},
		{DeltaText: "Hello"},
		{DeltaText: " world"},
		{DeltaToolCalls: []*exav1.ChatToolCall{
			{Id: "call_1", Name: "read_file", ArgumentsJson: `{"pa`},
		}},
		{DeltaToolCalls: []*exav1.ChatToolCall{
			{Id: "call_1", Name: "read_file", ArgumentsJson: `{"path":"/tmp"}`},
		}},
		{Usage: &exav1.ModelUsageStats{InputTokens: 10, OutputTokens: 4, CacheReadTokens: 6}},
	}, "{}", func(req *exav1.GetChatMessageRequest) { gotReq = req })
	defer srv.Close()

	p := &devin.Provider{BaseURL: srv.URL, HTTPClient: srv.Client(), CredentialLoader: devinTestCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model:          "swe-1-6",
		SystemPrompt:   "system prompt",
		Messages:       []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}},
		Tools:          []tools.Definition{{Name: "read_file", Description: "read", InputSchema: map[string]any{"type": "object"}}},
		ConversationID: "conv-1",
	}
	if err := p.RunTurn(context.Background(), req, events); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if gotReq == nil {
		t.Fatal("GetChatMessage was not called")
	}
	if gotReq.GetChatModelUid() != "swe-1-6" {
		t.Fatalf("chatModelUid: %q", gotReq.GetChatModelUid())
	}
	if gotReq.GetCascadeId() != "conv-1" {
		t.Fatalf("cascadeId: %q", gotReq.GetCascadeId())
	}
	if gotReq.GetPrompt() != "system prompt" {
		t.Fatalf("prompt: %q", gotReq.GetPrompt())
	}
	if gotReq.GetMetadata().GetUserJwt() != "user-jwt-1" {
		t.Fatalf("userJwt: %q", gotReq.GetMetadata().GetUserJwt())
	}
	if len(gotReq.GetTools()) != 1 || gotReq.GetTools()[0].GetName() != "read_file" {
		t.Fatalf("tools: %+v", gotReq.GetTools())
	}
	if len(gotReq.GetChatMessagePrompts()) != 1 {
		t.Fatalf("prompts: %+v", gotReq.GetChatMessagePrompts())
	}
	p0 := gotReq.GetChatMessagePrompts()[0]
	if p0.GetSource() != exav1.ChatMessageSource_CHAT_MESSAGE_SOURCE_USER || p0.GetPrompt() != "hi" {
		t.Fatalf("user prompt: %+v", p0)
	}
	if p0.GetMessageId() == "" {
		t.Fatal("user prompt missing deterministic messageId")
	}

	var got []loop.Event
	for {
		ev, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatalf("stream err: %v", err)
			}
			break
		}
		got = append(got, ev)
	}
	kinds := make([]loop.EventKind, len(got))
	for i, ev := range got {
		kinds[i] = ev.Kind
	}
	want := []loop.EventKind{loop.EventThinking, loop.EventText, loop.EventText, loop.EventToolCall, loop.EventUsage}
	if kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] || kinds[3] != want[3] || kinds[4] != want[4] {
		t.Fatalf("event kinds: %v", kinds)
	}
	if got[0].Text != "thinking..." || got[0].ThoughtSignature != "sig-1" {
		t.Fatalf("thinking event: %+v", got[0])
	}
	if got[3].Call == nil || got[3].Call.ID != "call_1" || got[3].Call.Name != "read_file" {
		t.Fatalf("tool call event: %+v", got[3].Call)
	}
	if string(got[3].Call.Arguments) != `{"path":"/tmp"}` {
		t.Fatalf("tool args (longest cumulative wins): %s", got[3].Call.Arguments)
	}
	if got[4].Usage == nil || got[4].Usage.Input != 10 || got[4].Usage.Output != 4 || got[4].Usage.CacheRead != 6 {
		t.Fatalf("usage event: %+v", got[4].Usage)
	}
}

func TestDevinProviderStreamTrailerError(t *testing.T) {
	srv := devinMockServer(t, nil, `{"error":{"code":"invalid_argument","message":"internal error"}}`, nil)
	defer srv.Close()

	p := &devin.Provider{BaseURL: srv.URL, HTTPClient: srv.Client(), CredentialLoader: devinTestCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{Model: "swe-1-6", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}}
	err := p.RunTurn(context.Background(), req, events)
	if err == nil {
		t.Fatal("expected stream error")
	}
	if !strings.Contains(err.Error(), "invalid_argument") {
		t.Fatalf("error: %v", err)
	}
}

// devinCatalogServer answers GetCliModelConfigs with the given configs.
func devinCatalogServer(t *testing.T, configs []*exav1.ClientModelConfig) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exa.api_server_pb.ApiServerService/GetCliModelConfigs" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		req := &exav1.GetCliModelConfigsRequest{}
		if err := proto.Unmarshal(raw, req); err != nil {
			t.Errorf("GetCliModelConfigs request decode: %v", err)
		}
		if !strings.HasPrefix(req.GetMetadata().GetApiKey(), "devin-session-token$") {
			t.Errorf("apiKey missing session prefix: %q", req.GetMetadata().GetApiKey())
		}
		resp, _ := proto.Marshal(&exav1.GetCliModelConfigsResponse{ClientModelConfigs: configs})
		w.Header().Set("content-type", "application/proto")
		_, _ = w.Write(resp)
	}))
}

func TestDevinDiscoveryNormalizesCatalog(t *testing.T) {
	srv := devinCatalogServer(t, []*exav1.ClientModelConfig{
		{ModelUid: "swe-1-6", MaxTokens: 200000, SupportsImages: true, ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_NORMAL}},
		{ModelUid: "disabled-model", Disabled: true},
		{ModelUid: "quick-review", ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_QUICK_REVIEW}},
		{ModelUid: "internal-default", ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_INTERNAL_DEFAULT}},
		{ModelUid: "adaptive", ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_MODEL_ROUTER}},
		{ModelUid: "fusion-gpt-5-sidekick-swe-1-6", MaxTokens: 1000, ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_NORMAL}},
		{ModelUid: "gpt-5", MaxTokens: 400000, SupportsImages: true, ModelInfo: &exav1.ModelInfo{DisplayOption: exav1.DisplayOption_DISPLAY_OPTION_NORMAL}},
	})
	defer srv.Close()

	models, _, err := devin.FetchModels(context.Background(), srv.Client(), srv.URL, "session-token")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}
	for _, id := range []string{"swe-1-6", "adaptive", "fusion-gpt-5-sidekick-swe-1-6", "gpt-5"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing model %q in %+v", id, models)
		}
	}
	for _, id := range []string{"disabled-model", "quick-review", "internal-default"} {
		if _, ok := byID[id]; ok {
			t.Fatalf("internal model %q leaked into catalog", id)
		}
	}
	// swe-1-6 is image-blind upstream: advertised but drops images.
	if models[byID["swe-1-6"]].SupportsImages {
		t.Fatal("swe-1-6 should be text-only (image-blind)")
	}
	// Fusion budgets against the lead's window, not the composite card's.
	if got := models[byID["fusion-gpt-5-sidekick-swe-1-6"]].ContextWindow; got != 400000 {
		t.Fatalf("fusion context window: %d", got)
	}
}
