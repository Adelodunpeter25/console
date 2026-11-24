// Claude cache warming replays the last request shortly before the cache
// entry expires, so an idle conversation keeps its cache.
package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
)

type warmServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func (w *warmServer) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.bodies)
}

func newWarmServer(t *testing.T, promptTokens int) *warmServer {
	t.Helper()
	w := &warmServer{}
	w.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.mu.Lock()
		w.bodies = append(w.bodies, string(raw))
		w.mu.Unlock()
		rw.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(rw, claudeSSEBody([][2]string{
			{"message_start", fmt.Sprintf(`{"message":{"usage":{"input_tokens":0,"cache_read_input_tokens":%d,"cache_creation_input_tokens":0}}}`, promptTokens)},
			{"content_block_delta", `{"index":0,"delta":{"type":"text_delta","text":"hi"}}`},
			{"message_delta", `{"usage":{"output_tokens":1}}`},
		}))
	}))
	t.Cleanup(w.Close)
	t.Cleanup(claude.StopAllWarming)
	return w
}

func runWarmTurn(t *testing.T, srv *warmServer, conversation string, retention loop.CacheRetention) {
	t.Helper()
	p := &claude.Provider{BaseURL: srv.URL, CredentialLoader: testClaudeCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model: "m", SystemPrompt: "sys", ConversationID: conversation, CacheRetention: retention,
		Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hello"}},
	}
	if err := p.RunTurn(context.Background(), req, events); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestClaudeWarmReplaysTheIdenticalRequest(t *testing.T) {
	defer claude.SetWarmTiming(40*time.Millisecond, time.Hour)()
	srv := newWarmServer(t, 50_000)
	runWarmTurn(t, srv, "conv-warm", loop.CacheLong)
	waitFor(t, "the warm request", func() bool { return srv.count() >= 2 })
	if srv.bodies[0] != srv.bodies[1] {
		t.Fatal("the warm must replay the exact request so it reads the same cached prefix")
	}
}

func TestClaudeRealRequestCancelsThePendingWarm(t *testing.T) {
	defer claude.SetWarmTiming(150*time.Millisecond, time.Hour)()
	srv := newWarmServer(t, 50_000)
	runWarmTurn(t, srv, "conv-cancel", loop.CacheLong)
	runWarmTurn(t, srv, "conv-cancel", loop.CacheLong)
	time.Sleep(100 * time.Millisecond)
	if got := srv.count(); got != 2 {
		t.Fatalf("requests = %d, want just the two real ones before the timer", got)
	}
}

func TestClaudeSkipsWarmingWhenNotWorthIt(t *testing.T) {
	defer claude.SetWarmTiming(20*time.Millisecond, time.Hour)()
	small := newWarmServer(t, 1_000)
	runWarmTurn(t, small, "conv-small", loop.CacheLong)
	short := newWarmServer(t, 50_000)
	runWarmTurn(t, short, "conv-short", loop.CacheShort)
	time.Sleep(150 * time.Millisecond)
	if small.count() != 1 || short.count() != 1 {
		t.Fatalf("small prompt requests = %d, short retention requests = %d; neither should be warmed", small.count(), short.count())
	}
}

func TestClaudeWarmingStopsAfterTheIdleLimit(t *testing.T) {
	defer claude.SetWarmTiming(30*time.Millisecond, 50*time.Millisecond)()
	srv := newWarmServer(t, 50_000)
	runWarmTurn(t, srv, "conv-idle", loop.CacheLong)
	waitFor(t, "the first warm", func() bool { return srv.count() >= 2 })
	time.Sleep(250 * time.Millisecond)
	if got := srv.count(); got > 3 {
		t.Fatalf("requests = %d; warming must stop once the idle limit is reached", got)
	}
}
