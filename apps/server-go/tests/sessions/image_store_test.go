// Stage 5: pasted images and annotation screenshots live as files named by
// their content hash, beside the session database, instead of inside it.
package tests

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// diskStore opens a real on-disk storage directory, which is the only setup in
// which images are written to files.
func diskStore(t *testing.T) (*services.SessionService, *db.DB, string) {
	t.Helper()
	dir := t.TempDir()
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return services.NewSessionService(manager), manager, dir
}

func diskSession(t *testing.T, s *services.SessionService, project *string) types.SessionHeader {
	t.Helper()
	h, err := s.Create(types.CreateSessionOptions{Title: "t", Cwd: t.TempDir(), ModelID: "m", Provider: "p", ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

var pngBytes = []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("pixel", 4000))

func imageMsg(id, text string, images ...[]byte) types.AgentMessage {
	atts := make([]map[string]string, 0, len(images))
	for _, img := range images {
		atts = append(atts, map[string]string{"data": base64.StdEncoding.EncodeToString(img), "mimeType": "image/png"})
	}
	data, _ := json.Marshal(map[string]any{"user": map[string]any{"content": text, "attachments": atts}})
	return types.AgentMessage{ID: id, Role: "user", Data: data}
}

func imagesDir(dir string, h types.SessionHeader) string {
	proj := ""
	if h.ProjectID != nil {
		proj = *h.ProjectID
	}
	if proj == "" {
		return filepath.Join(dir, "projects", "scratch", "sessions", h.ID+".images")
	}
	return filepath.Join(dir, "projects", proj, "sessions", h.ID+".images")
}

func fileNames(t *testing.T, d string) []string {
	t.Helper()
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestImageBodyGoesToAFileAndLeavesTheDatabase(t *testing.T) {
	s, m, dir := diskStore(t)
	h := diskSession(t, s, nil)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "look", pngBytes)}); err != nil {
		t.Fatal(err)
	}
	files := fileNames(t, imagesDir(dir, h))
	if len(files) != 1 || len(files[0]) != 64 {
		t.Fatalf("expected one digest-named file, got %v", files)
	}
	got, err := os.ReadFile(filepath.Join(imagesDir(dir, h), files[0]))
	if err != nil || string(got) != string(pngBytes) {
		t.Fatalf("the file must hold the raw image bytes: %v", err)
	}
	// The database part carries a reference, not the pixels.
	c := sessionConn(t, m, h.ID)
	var meta string
	var body []byte
	if err := c.QueryRow(`SELECT meta, body FROM message_parts WHERE kind = 'image'`).Scan(&meta, &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 || !strings.Contains(meta, files[0]) {
		t.Fatalf("the part should hold only the reference: body=%d bytes, meta=%s", len(body), meta)
	}
	// content is still complete, so nothing depends on the file yet.
	var content string
	_ = c.QueryRow(`SELECT content FROM messages WHERE id = 'u1'`).Scan(&content)
	if !strings.Contains(content, base64.StdEncoding.EncodeToString(pngBytes)) {
		t.Fatal("content must still carry the image during the transition")
	}
}

func TestLoadServesTheImageBackFromTheFile(t *testing.T) {
	s, _, _ := diskStore(t)
	h := diskSession(t, s, nil)
	msg := imageMsg("u1", "look", pngBytes)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{msg}); err != nil {
		t.Fatal(err)
	}
	checkedBefore, badBefore := session.ParityStats()
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 1 {
		t.Fatalf("load: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(loaded.Messages[0], &got)
	delete(got, "id")
	delete(got, "createdAt")
	gotRaw, _ := json.Marshal(got)
	if same, _ := msgparts.Equal(msg.Data, gotRaw); !same {
		t.Fatalf("the image did not come back identical")
	}
	if checked, bad := session.ParityStats(); checked == checkedBefore || bad != badBefore {
		t.Fatalf("the rebuild from the file should have been checked and clean: checked %d->%d bad %d->%d", checkedBefore, checked, badBefore, bad)
	}
}

func TestTheSameImageIsStoredOnce(t *testing.T) {
	s, _, dir := diskStore(t)
	h := diskSession(t, s, nil)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{
		imageMsg("u1", "first", pngBytes),
		imageMsg("u2", "again", pngBytes),
		imageMsg("u3", "and again", pngBytes, []byte("a different image")),
	}); err != nil {
		t.Fatal(err)
	}
	if files := fileNames(t, imagesDir(dir, h)); len(files) != 2 {
		t.Fatalf("identical images must share one file: %v", files)
	}
}

// If the file is gone or damaged, the original content is served: history is
// never wrong, and the problem is counted.
func TestAMissingOrDamagedImageFileFallsBackToTheOriginal(t *testing.T) {
	for name, damage := range map[string]func(path string){
		"deleted":   func(p string) { os.Remove(p) },
		"truncated": func(p string) { os.WriteFile(p, []byte("short"), 0o644) },
		"replaced":  func(p string) { os.WriteFile(p, []byte(strings.Repeat("x", len(pngBytes))), 0o644) },
		"a symlink": func(p string) { os.Remove(p); os.Symlink("/etc/hosts", p) },
		"a folder":  func(p string) { os.Remove(p); os.Mkdir(p, 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			s, m, dir := diskStore(t)
			h := diskSession(t, s, nil)
			msg := imageMsg("u1", "look", pngBytes)
			if err := s.AppendMessages(h.ID, []types.AgentMessage{msg}); err != nil {
				t.Fatal(err)
			}
			files := fileNames(t, imagesDir(dir, h))
			damage(filepath.Join(imagesDir(dir, h), files[0]))

			fresh := services.NewSessionService(m) // empty verification cache
			_, badBefore := session.ParityStats()
			loaded, err := fresh.Load(h.ID, 0, 0)
			if err != nil {
				t.Fatalf("a damaged image file must not fail the load: %v", err)
			}
			var got map[string]any
			_ = json.Unmarshal(loaded.Messages[0], &got)
			delete(got, "id")
			delete(got, "createdAt")
			gotRaw, _ := json.Marshal(got)
			if same, _ := msgparts.Equal(msg.Data, gotRaw); !same {
				t.Fatal("served wrong history instead of falling back to the original")
			}
			if _, badAfter := session.ParityStats(); badAfter == badBefore {
				t.Fatal("the failure was not counted")
			}
		})
	}
}

// A forged reference must never be joined into a path.
func TestAForgedImageReferenceCannotEscapeTheFolder(t *testing.T) {
	s, m, dir := diskStore(t)
	h := diskSession(t, s, nil)
	msg := imageMsg("u1", "look", pngBytes)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{msg}); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	for name, ref := range map[string]string{
		"traversal": "../../../../secret.txt",
		"absolute":  secret,
		"short":     "abc",
		"uppercase": strings.ToUpper(strings.Repeat("a", 64)),
		"slashes":   strings.Repeat("a", 30) + "/" + strings.Repeat("b", 33),
	} {
		forged, _ := json.Marshal(map[string]any{"mime": "image/png", "enc": "base64", "ref": ref})
		if _, err := c.Exec(`UPDATE message_parts SET meta = ?, body = NULL WHERE kind = 'image'`, string(forged)); err != nil {
			t.Fatal(err)
		}
		fresh := services.NewSessionService(m)
		loaded, err := fresh.Load(h.ID, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(string(loaded.Messages[0]), "TOP SECRET") || strings.Contains(string(loaded.Messages[0]), base64.StdEncoding.EncodeToString([]byte("TOP SECRET"))) {
			t.Fatalf("%s: a forged reference read a file outside the image folder", name)
		}
	}
}

func TestRewritingHistoryRemovesImagesNothingNeedsAnymore(t *testing.T) {
	session.ImageSweepGrace = 0
	t.Cleanup(func() { session.ImageSweepGrace = time.Hour })
	s, _, dir := diskStore(t)
	h := diskSession(t, s, nil)
	other := []byte("the image that stays")
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "a", pngBytes), imageMsg("u2", "b", other)}); err != nil {
		t.Fatal(err)
	}
	if len(fileNames(t, imagesDir(dir, h))) != 2 {
		t.Fatal("setup: expected two image files")
	}
	// Rewrite history keeping only the second image.
	if err := s.ReplaceMessages(h.ID, []types.AgentMessage{imageMsg("u2", "b", other)}); err != nil {
		t.Fatal(err)
	}
	files := fileNames(t, imagesDir(dir, h))
	if len(files) != 1 {
		t.Fatalf("the unreferenced image should be swept, the used one kept: %v", files)
	}
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 1 || !strings.Contains(string(loaded.Messages[0]), base64.StdEncoding.EncodeToString(other)) {
		t.Fatalf("the kept image must still load: %v", err)
	}
}

// A file written a moment ago may belong to a message still being saved; the
// sweep leaves anything younger than the grace period alone.
func TestTheSweepSparesFreshFilesAndForeignFiles(t *testing.T) {
	session.ImageSweepGrace = time.Hour
	s, _, dir := diskStore(t)
	h := diskSession(t, s, nil)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "a", pngBytes)}); err != nil {
		t.Fatal(err)
	}
	folder := imagesDir(dir, h)
	foreign := filepath.Join(folder, "notes.txt") // not something the store creates
	if err := os.WriteFile(foreign, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	_ = os.Chtimes(foreign, old, old)
	if err := s.ReplaceMessages(h.ID, []types.AgentMessage{{ID: "n", Role: "user", Data: []byte(`{"user":{"content":"x"}}`)}}); err != nil {
		t.Fatal(err)
	}
	names := fileNames(t, folder)
	if len(names) != 2 {
		t.Fatalf("a fresh unreferenced file and a foreign file must both survive: %v", names)
	}
}

func TestDeletingASessionDeletesItsImages(t *testing.T) {
	s, _, dir := diskStore(t)
	h := diskSession(t, s, nil)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "a", pngBytes)}); err != nil {
		t.Fatal(err)
	}
	folder := imagesDir(dir, h)
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if ok, err := s.SoftDelete(h.ID); err != nil || !ok {
		t.Fatalf("soft delete: %v %v", ok, err)
	}
	if ok, err := s.PermanentDelete(h.ID); err != nil || !ok {
		t.Fatalf("permanent delete: %v %v", ok, err)
	}
	if _, err := os.Stat(folder); err == nil {
		t.Fatal("the images folder outlived its session")
	}
}

func TestForkCopiesImagesIntoItsOwnFolder(t *testing.T) {
	s, _, dir := diskStore(t)
	h := diskSession(t, s, nil)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "a", pngBytes)}); err != nil {
		t.Fatal(err)
	}
	fork, err := s.Fork(h.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if files := fileNames(t, imagesDir(dir, fork)); len(files) != 1 {
		t.Fatalf("the fork needs its own copy of the image: %v", files)
	}
	// Deleting the original must not break the fork.
	_, _ = s.SoftDelete(h.ID)
	_, _ = s.PermanentDelete(h.ID)
	loaded, err := s.Load(fork.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 1 || !strings.Contains(string(loaded.Messages[0]), base64.StdEncoding.EncodeToString(pngBytes)) {
		t.Fatalf("the fork lost its image when the original was deleted: %v", err)
	}
}

func TestBackfillMovesOldImagesToFiles(t *testing.T) {
	s, m, dir := diskStore(t)
	h := diskSession(t, s, nil)
	c := sessionConn(t, m, h.ID)
	legacy := imageMsg("old1", "old", pngBytes)
	if _, err := c.Exec(`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, 1)`, legacy.ID, legacy.Role, string(legacy.Data)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(h.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if files := fileNames(t, imagesDir(dir, h)); len(files) != 1 {
		t.Fatalf("backfill should have written the old image to a file: %v", files)
	}
	verifyImageSplit(t, c, "old1", imagesDir(dir, h))
}

// The image still being safe if its folder moves with the session between projects.
func TestMovingASessionBetweenProjectsMovesItsImages(t *testing.T) {
	s, m, dir := diskStore(t)
	h := diskSession(t, s, nil) // scratch
	if err := s.AppendMessages(h.ID, []types.AgentMessage{imageMsg("u1", "a", pngBytes)}); err != nil {
		t.Fatal(err)
	}
	proj := t.TempDir()
	info, err := projectssvc.NewProjectService(m).Create(projectssvc.CreateProjectOptions{Name: "P", Dir: proj})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	p := info.ID
	if err := s.UpdateCwd(h.ID, proj, &p, true); err != nil {
		t.Fatal(err)
	}
	moved := h
	moved.ProjectID = &p
	if files := fileNames(t, imagesDir(dir, moved)); len(files) != 1 {
		t.Fatalf("the image folder must follow the session: %v", files)
	}
	if left := fileNames(t, imagesDir(dir, h)); len(left) != 0 {
		t.Fatalf("the old location still holds images: %v", left)
	}
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 1 || !strings.Contains(string(loaded.Messages[0]), base64.StdEncoding.EncodeToString(pngBytes)) {
		t.Fatalf("the image did not load after the move: %v", err)
	}
}

func verifyImageSplit(t *testing.T, c *sql.DB, id, imgDir string) {
	t.Helper()
	var content, skeleton string
	if err := c.QueryRow(`SELECT content, skeleton FROM messages WHERE id = ?`, id).Scan(&content, &skeleton); err != nil {
		t.Fatal(err)
	}
	rows, _ := c.Query(`SELECT slot, kind, meta, body FROM message_parts WHERE message_id = ? ORDER BY slot`, id)
	defer rows.Close()
	var parts []msgparts.Part
	for rows.Next() {
		var p msgparts.Part
		var meta string
		_ = rows.Scan(&p.Slot, &p.Kind, &meta, &p.Body)
		_ = json.Unmarshal([]byte(meta), &p.Meta)
		if p.Meta.Ref != "" {
			b, err := os.ReadFile(filepath.Join(imgDir, p.Meta.Ref))
			if err != nil {
				t.Fatal(err)
			}
			p.Body = b
		}
		parts = append(parts, p)
	}
	back, err := msgparts.Assemble([]byte(skeleton), parts)
	if err != nil {
		t.Fatal(err)
	}
	if same, _ := msgparts.Equal([]byte(content), back); !same {
		t.Fatal("an image message does not rebuild from its skeleton, parts and file")
	}
}
