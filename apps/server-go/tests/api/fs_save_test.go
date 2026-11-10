// Atomic, conflict-checked saving: POST /api/fs/save must preserve the bytes
// it is given, survive the target's permissions and symlinks, refuse stale
// buffers, and leave no temp files behind.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

type saveResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
	Etag    string `json:"etag"`
	Data    struct {
		Path      string `json:"path"`
		Etag      string `json:"etag"`
		SizeBytes string `json:"sizeBytes"`
		MtimeMs   string `json:"mtimeMs"`
	} `json:"data"`
}

// saveBody marshals a save request so etags (which contain quotes) and
// embedded newlines stay valid JSON.
func saveBody(t *testing.T, path, content, baseEtag string) string {
	t.Helper()
	body := map[string]string{"path": path, "content": content}
	if baseEtag != "" {
		body["baseEtag"] = baseEtag
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func saveFile(t *testing.T, app *fiber.App, body string) (int, saveResp) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/fs/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out saveResp
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %d %s: %v", resp.StatusCode, raw, err)
	}
	return resp.StatusCode, out
}

func readEtag(t *testing.T, app *fiber.App, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/fs/file?path="+url.QueryEscape(path), nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	return resp.Header.Get("ETag")
}

func tempLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			found = append(found, e.Name())
		}
	}
	return found
}

func TestSaveFileRoundTripsAndKeepsCRLF(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")

	content := "line one\r\nline two\r\n"
	code, body := saveFile(t, app, saveBody(t, path, "line one\r\nline two\r\n", ""))
	if code != 200 || !body.Success {
		t.Fatalf("save: %d %+v", code, body)
	}
	if body.Data.Etag == "" {
		t.Fatalf("save must return an etag: %+v", body.Data)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != content {
		t.Fatalf("CRLF must be preserved verbatim: %q", raw)
	}
	if body.Data.Etag != readEtag(t, app, path) {
		t.Fatalf("returned etag must match GET /api/fs/file: %q vs %q", body.Data.Etag, readEtag(t, app, path))
	}
	if leftovers := tempLeftovers(t, root); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestSaveFileKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	app := fsRoutesApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "script.sh")
	if err := os.WriteFile(path, []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, body := saveFile(t, app, saveBody(t, path, "new\n", ""))
	if code != 200 || !body.Success {
		t.Fatalf("save: %d %+v", code, body)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("permissions changed: %v", info.Mode().Perm())
	}
}

func TestSaveFileFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	app := fsRoutesApp(t)
	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	code, body := saveFile(t, app, saveBody(t, link, "after\n", ""))
	if code != 200 || !body.Success {
		t.Fatalf("save: %d %+v", code, body)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link must stay a symlink: %v %v", info, err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "after\n" {
		t.Fatalf("target must hold new content: %q %v", raw, err)
	}
}

func TestSaveFileMissingParentCreatesNothing(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	missingDir := filepath.Join(root, "nope")
	path := filepath.Join(missingDir, "a.txt")

	code, body := saveFile(t, app, saveBody(t, path, "x\n", ""))
	if code != 400 || body.Success {
		t.Fatalf("missing parent must 4xx: %d %+v", code, body)
	}
	if _, err := os.Stat(missingDir); !os.IsNotExist(err) {
		t.Fatalf("must not create the parent directory: %v", err)
	}
}

func TestSaveFileConflictCheck(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if code, _ := saveFile(t, app, saveBody(t, path, "v1\n", "")); code != 200 {
		t.Fatalf("initial save: %d", code)
	}
	etag := readEtag(t, app, path)

	// Matching base_etag succeeds.
	code, body := saveFile(t, app, saveBody(t, path, "v2\n", etag))
	if code != 200 || !body.Success {
		t.Fatalf("matching etag must save: %d %+v", code, body)
	}

	// A stale etag loses; the file keeps its content.
	stale := etag
	code, body = saveFile(t, app, saveBody(t, path, "clobbered\n", stale))
	if code != 409 || body.Code != "FILE_CHANGED" {
		t.Fatalf("stale etag must 409 FILE_CHANGED: %d %+v", code, body)
	}
	if body.Etag == "" {
		t.Fatalf("409 must carry the current etag: %+v", body)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "v2\n" {
		t.Fatalf("file must be unchanged: %q %v", raw, err)
	}
	if leftovers := tempLeftovers(t, root); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestSaveFileFailureLeavesOriginal(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if code, _ := saveFile(t, app, saveBody(t, path, "original\n", "")); code != 200 {
		t.Fatalf("initial save failed")
	}

	// The target directory becoming read-only is the portable way to force a
	// write failure: the temp file cannot be created, so the rename never runs.
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions")
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o755) })

	code, body := saveFile(t, app, saveBody(t, path, "replaced\n", ""))
	if code == 200 || body.Success {
		t.Fatalf("write into a read-only directory must fail: %d %+v", code, body)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "original\n" {
		t.Fatalf("original must survive a failed save: %q %v", raw, err)
	}
	if leftovers := tempLeftovers(t, root); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestSaveFileRequiresPath(t *testing.T) {
	app := fsRoutesApp(t)
	if code, body := saveFile(t, app, `{"content":"x"}`); code != 400 || body.Success {
		t.Fatalf("missing path must 400: %d %+v", code, body)
	}
}
