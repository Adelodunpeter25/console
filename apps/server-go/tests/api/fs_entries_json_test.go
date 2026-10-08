// The hand-written /api/fs/entries encoder must be byte-for-byte what the old
// protojson path produced for flat entries, and must never emit `children`.
package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// legacyEntriesBody reproduces the previous encoding: protojson per entry,
// re-compacted by json.Marshal, wrapped in the envelope.
func legacyEntriesBody(t testing.TB, entries []types.FsTreeEntry) string {
	t.Helper()
	elements := make([]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		msg := &consolev1.FsTreeEntry{Name: e.Name, Path: e.Path, IsDir: e.IsDir}
		if e.Size != nil {
			v := uint64(*e.Size)
			msg.Size = &v
		}
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, raw)
	}
	data, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	return `{"success":true,"data":` + string(data) + `}`
}

func int64p(v int64) *int64 { return &v }

func TestFsEntriesEncoderMatchesProtojson(t *testing.T) {
	entries := []types.FsTreeEntry{
		{Name: "src", Path: "/p/src", IsDir: true},
		{Name: "README.md", Path: "/p/README.md", Size: int64p(42)},
		{Name: "empty", Path: "/p/empty", Size: int64p(0)},
		{Name: "big", Path: "/p/big", Size: int64p(9007199254740993)},
		{Name: `quo"te\back`, Path: `/p/quo"te\back`},
		{Name: "tab\there\nnl\rcr\b\f", Path: "/p/ctl"},
		{Name: "bell\x07\x1f", Path: "/p/low"},
		{Name: "héllo 日本語 🚀", Path: "/p/héllo 日本語 🚀", Size: int64p(7)},
		{Name: "</script>&<>", Path: "/p/html"},
		{Name: "\u2028\u2029", Path: "/p/sep"},
	}
	got := string(routes.EncodeFsEntriesResponse(entries))
	want := legacyEntriesBody(t, entries)
	if got != want {
		t.Fatalf("encoder drifted from protojson:\n got %s\nwant %s", got, want)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("invalid JSON: %s", got)
	}
}

func TestFsEntriesEncoderDropsChildren(t *testing.T) {
	entries := []types.FsTreeEntry{{
		Name: "src", Path: "/p/src", IsDir: true,
		Children: []types.FsTreeEntry{{Name: "a.go", Path: "/p/src/a.go"}},
	}}
	got := string(routes.EncodeFsEntriesResponse(entries))
	if strings.Contains(got, "children") || strings.Contains(got, "a.go") {
		t.Fatalf("children must not be encoded: %s", got)
	}
}

func TestFsEntriesEncoderEmptyAndInvalidUTF8(t *testing.T) {
	if got := string(routes.EncodeFsEntriesResponse(nil)); got != `{"success":true,"data":[]}` {
		t.Fatalf("empty: %s", got)
	}
	// A non-UTF-8 filename used to fail the whole listing; it is now replaced.
	got := routes.EncodeFsEntriesResponse([]types.FsTreeEntry{{Name: "bad\xff.txt", Path: "/p/bad\xff.txt"}})
	if !json.Valid(got) || !strings.Contains(string(got), `bad\ufffd.txt`) {
		t.Fatalf("invalid utf8 handling: %s", got)
	}
}

func TestFsEntriesRouteIsFlatAndClientParsable(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"src/a.go", "src/deep/b.go", "top.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("GET", "/api/fs/entries?path="+url.QueryEscape(root), nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type: %q", ct)
	}
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "children") {
		t.Fatalf("route leaked children: %s", raw)
	}
	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || !env.Success {
		t.Fatalf("envelope: %v %s", err, raw)
	}
	seen := map[string]bool{}
	for _, e := range env.Data {
		seen[e.Name] = true
	}
	for _, want := range []string{"src", "a.go", "deep", "b.go", "top.txt"} {
		if !seen[want] {
			t.Fatalf("missing %s in flat listing: %s", want, raw)
		}
	}
}

func benchEntries(n int) []types.FsTreeEntry {
	out := make([]types.FsTreeEntry, 0, n)
	for i := 0; i < n; i++ {
		size := int64(i * 37)
		out = append(out, types.FsTreeEntry{
			Name: fmt.Sprintf("file_%d.go", i),
			Path: fmt.Sprintf("/Users/dev/project/apps/server-go/internal/services/file_%d.go", i),
			Size: &size,
		})
	}
	return out
}

func BenchmarkFsEntriesLegacy(b *testing.B) {
	entries := benchEntries(1700)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = legacyEntriesBody(b, entries)
	}
}

func BenchmarkFsEntriesHandWritten(b *testing.B) {
	entries := benchEntries(1700)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = routes.EncodeFsEntriesResponse(entries)
	}
}
