// Filesystem migration coverage: proto-built payloads must match the shared
// golden fixtures, and the file/dir CRUD routes keep their semantics.
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func fsFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "fs", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func checkFsFixture(t *testing.T, name string, msg proto.Message) {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != fsFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), fsFixture(t, name))
	}
}

func checkFsArrayFixture(t *testing.T, name string, items []proto.Message) {
	t.Helper()
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protojson.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, json.RawMessage(compactJSON(t, raw)))
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != fsFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, encoded, fsFixture(t, name))
	}
}

func TestFsProtoMatchesFixtures(t *testing.T) {
	size1234 := uint64(1234)
	size42 := uint64(42)
	checkFsFixture(t, "browse.json", &consolev1.FsBrowseResult{
		CurrentPath: "/tmp/proj", ParentPath: strptr("/tmp"),
		Entries: []*consolev1.FsTreeEntry{
			{Name: "src", Path: "/tmp/proj/src", IsDir: true},
			{Name: "main.go", Path: "/tmp/proj/main.go", Size: &size1234},
		},
	})
	checkFsArrayFixture(t, "entries.json", []proto.Message{
		&consolev1.FsTreeEntry{
			Name: "src", Path: "/p/src", IsDir: true,
			Children: []*consolev1.FsTreeEntry{
				{Name: "a.go", Path: "/p/src/a.go"},
			},
		},
		&consolev1.FsTreeEntry{Name: "README.md", Path: "/p/README.md", Size: &size42},
	})
	checkFsArrayFixture(t, "search.json", []proto.Message{
		&consolev1.FileSearchResult{
			RelativePath: "src/a.go", AbsolutePath: "/p/src/a.go", Score: 0.95,
		},
	})
	checkFsFixture(t, "grep.json", &consolev1.GrepResult{
		Matches: []*consolev1.GrepMatch{{
			RelPath: "src/a.go", LineNumber: 12, LineContent: "foo bar",
			MatchRanges: []*consolev1.GrepMatchRange{{Start: 4, End: 7}},
		}},
		TotalMatched: 3, FilteredFiles: 1, NextCursor: 200, HasMore: true,
	})
	checkFsFixture(t, "tree.json", &consolev1.FsDirectoryTree{
		Path: "/p", TreeFormatted: "Directory: /p (recursive, max depth 3)\n└── a.go\n",
	})
	checkFsFixture(t, "file_content.json", &consolev1.FsFileContent{
		Path: "/p/a.go", Content: "package main\n",
	})
	checkFsFixture(t, "file_write.json", &consolev1.FileWriteResponse{
		Path: "/p/a.go", Message: "Wrote /p/a.go (12 bytes)",
	})
	checkFsFixture(t, "file_delete.json", &consolev1.FileDeleteResponse{
		Path: "/p/a.go", Deleted: true,
	})
	checkFsFixture(t, "dir_create.json", &consolev1.DirCreateResponse{
		Path: "/p/new", Created: true,
	})
	checkFsFixture(t, "dir_delete.json", &consolev1.DirDeleteResponse{
		Path: "/p/new", Deleted: true,
	})
	checkFsFixture(t, "watch.json", &consolev1.FsChangeEvent{
		Type: "fsChange", ProjectPath: "/p", EventPath: strptr("/p/a.go"),
	})

	// Fixtures stay parseable with unknown-field tolerance.
	var decoded consolev1.GrepResult
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(fsFixture(t, "grep.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetTotalMatched() != 3 || len(decoded.GetMatches()) != 1 {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func fsRoutesApp(t *testing.T) *fiber.App {
	t.Helper()
	watch, err := services.NewFsWatchService()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(watch.Close)
	app := fiber.New()
	routes.RegisterFsRoutes(app, services.NewFsService(), watch)
	return app
}

func fsEnvelope(t *testing.T, raw []byte) (bool, json.RawMessage) {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope: %s", raw)
	}
	return envelope.Success, envelope.Data
}

func TestFsRoutesFiles(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()

	call := func(method, target, body string) (int, []byte) {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, target, nil)
		} else {
			req = httptest.NewRequest(method, target, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}
	get := func(path string) (int, []byte) {
		return call("GET", "/api/fs/"+path+"?path="+url.QueryEscape(root), "")
	}
	getFile := func(path string) (int, []byte) {
		return call("GET", "/api/fs/file?path="+url.QueryEscape(path), "")
	}

	// Write then read back.
	filePath := filepath.Join(root, "a.go")
	if code, _ := call("POST", "/api/fs/file", `{"path":"`+filePath+`","content":"package main\n"}`); code != 200 {
		t.Fatalf("write: %d", code)
	}
	if code, raw := getFile(filePath); code != 200 {
		t.Fatalf("read: %d %s", code, raw)
	} else {
		ok, data := fsEnvelope(t, raw)
		if !ok {
			t.Fatalf("envelope: %s", raw)
		}
		var content struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(data, &content); err != nil || content.Content != "package main\n" {
			t.Fatalf("content: %s", data)
		}
	}

	// Browse shows the file with a string size; tree renders its name.
	if code, raw := get("browse"); code != 200 {
		t.Fatalf("browse: %d %s", code, raw)
	} else {
		ok, data := fsEnvelope(t, raw)
		if !ok {
			t.Fatalf("envelope: %s", raw)
		}
		var result struct {
			Entries []map[string]any `json:"entries"`
		}
		if err := json.Unmarshal(data, &result); err != nil || len(result.Entries) != 1 {
			t.Fatalf("entries: %s", data)
		}
		if _, isString := result.Entries[0]["size"].(string); !isString {
			t.Fatalf("size must be a string: %v", result.Entries[0])
		}
	}
	if code, raw := get("tree"); code != 200 || !strings.Contains(string(raw), "a.go") {
		t.Fatalf("tree: %d %s", code, raw)
	}

	// Dir lifecycle.
	dirPath := filepath.Join(root, "new")
	if code, _ := call("POST", "/api/fs/dir", `{"path":"`+dirPath+`"}`); code != 200 {
		t.Fatalf("mkdir: %d", code)
	}
	if code, _ := call("DELETE", "/api/fs/dir?path="+url.QueryEscape(dirPath), ""); code != 200 {
		t.Fatalf("rmdir: %d", code)
	}
	if code, _ := call("DELETE", "/api/fs/file?path="+url.QueryEscape(filePath), ""); code != 200 {
		t.Fatalf("rm: %d", code)
	}

	// Validation preserved.
	if code, _ := call("GET", "/api/fs/file", ""); code != 400 {
		t.Fatalf("missing path must 400: %d", code)
	}
	if code, _ := call("POST", "/api/fs/file", `{}`); code != 400 {
		t.Fatalf("missing body path must 400: %d", code)
	}
}
