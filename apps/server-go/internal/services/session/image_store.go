// Image store: pasted images and annotation screenshots are 15% of a real
// database and are re-read with every message row, yet nothing ever searches
// them. Each is written once as a file named by the SHA-256 of its bytes, and
// the message part keeps only that name.
//
// The folder is per session, next to the session database, and shares its
// lifetime. That is deliberate: a store shared across sessions would need
// reference counting across separate databases, and a crash between writing a
// file and recording a reference could delete an image a message still needs.
// Here, deleting a session deletes its folder, and the only cleanup needed is
// sweeping files a rewritten history no longer refers to.
//
// A failure of the store never fails a write or a read of a message: the body
// stays in the database instead, and a file that cannot be read makes the read
// fall back to the original content.
package session

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ImageSweepGrace is how old an unreferenced file must be before a sweep may
// delete it. A write puts the file down before its row commits, so a younger
// file may belong to a message still being saved.
var ImageSweepGrace = time.Hour

type imageStore struct{ dir string }

// newImageStore returns the store for a session, or nil when there is nowhere
// to put files (an in-memory database), in which case bodies stay inline.
func newImageStore(storageDir, projectID, sessionID string) *imageStore {
	if storageDir == "" || storageDir == ":memory:" {
		return nil
	}
	return &imageStore{dir: utils.SessionImagesDir(storageDir, projectID, sessionID)}
}

func isImageKind(kind string) bool {
	return kind == msgparts.KindImage || kind == msgparts.KindScreenshot
}

// validRef accepts only a 64-character lowercase hex digest. The ref comes from
// a database row, so it is checked before it is ever joined into a path.
func validRef(ref string) bool {
	if len(ref) != 64 {
		return false
	}
	for _, c := range ref {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// put stores body and returns its ref. Writing is atomic (temp file, then
// rename), and a file that already holds these bytes is left alone.
func (st *imageStore) put(body []byte) (string, error) {
	sum := sha256.Sum256(body)
	ref := hex.EncodeToString(sum[:])
	path := filepath.Join(st.dir, ref)
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(body)) {
		return ref, nil
	}
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(st.dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return "", err
	}
	return ref, nil
}

// get reads the bytes a ref names.
func (st *imageStore) get(ref string) ([]byte, error) {
	if !validRef(ref) {
		return nil, fmt.Errorf("image reference %q is not a valid digest", ref)
	}
	path := filepath.Join(st.dir, ref)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image file %s is not a regular file", ref)
	}
	return os.ReadFile(path)
}

// externalize moves the body of every image part into the store, leaving a ref.
// A part the store cannot take keeps its body, so the message is still complete.
func (st *imageStore) externalize(parts []msgparts.Part) []msgparts.Part {
	if st == nil {
		return parts
	}
	out := make([]msgparts.Part, len(parts))
	copy(out, parts)
	for i, p := range out {
		if !isImageKind(p.Kind) || len(p.Body) == 0 {
			continue
		}
		ref, err := st.put(p.Body)
		if err != nil {
			// Keep the body in the database; do not fail the message.
			continue
		}
		out[i].Meta.Ref = ref
		out[i].Body = nil
	}
	return out
}

// hydrate fills Body from the store for every part that has a Ref.
func (st *imageStore) hydrate(parts []msgparts.Part) error {
	for i, p := range parts {
		if p.Meta.Ref == "" {
			continue
		}
		if st == nil {
			return fmt.Errorf("part %d is stored as a file but this session has no image store", p.Slot)
		}
		body, err := st.get(p.Meta.Ref)
		if err != nil {
			return fmt.Errorf("part %d: %w", p.Slot, err)
		}
		parts[i].Body = body
	}
	return nil
}

// refsInUse is every ref the session's parts still point at.
func refsInUse(queryRows func() ([]string, error)) (map[string]bool, error) {
	metas, err := queryRows()
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool)
	for _, raw := range metas {
		var m msgparts.Meta
		if json.Unmarshal([]byte(raw), &m) == nil && m.Ref != "" {
			used[m.Ref] = true
		}
	}
	return used, nil
}

// sweep deletes files nothing refers to and that are older than the grace
// period. It only ever touches names the store creates (a digest, or a stale
// temp file), so an unrelated file placed in the folder is never removed.
func (st *imageStore) sweep(used map[string]bool, grace time.Duration) (removed int, err error) {
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-grace)
	for _, e := range entries {
		name := e.Name()
		ours := validRef(name) || strings.HasPrefix(name, ".tmp-")
		if !ours || !e.Type().IsRegular() || used[name] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(st.dir, name)) == nil {
			removed++
		}
	}
	return removed, nil
}

// sweepImages deletes this session's image files that no part refers to any
// more. It is best effort: a failure only leaves files behind for the next sweep.
func (s *Service) sweepImages(sessionID, projectID string, conn *sql.DB) {
	st := s.imagesFor(sessionID, projectID)
	if st == nil {
		return
	}
	used, err := refsInUse(func() ([]string, error) {
		rows, err := conn.Query(`SELECT meta FROM message_parts WHERE meta LIKE '%"ref"%'`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var metas []string
		for rows.Next() {
			var m string
			if err := rows.Scan(&m); err != nil {
				return nil, err
			}
			metas = append(metas, m)
		}
		return metas, rows.Err()
	})
	if err != nil {
		slog.Warn("image sweep skipped", "session", sessionID, "error", err)
		return
	}
	if n, err := st.sweep(used, ImageSweepGrace); err != nil {
		slog.Warn("image sweep failed", "session", sessionID, "error", err)
	} else if n > 0 {
		slog.Info("image sweep removed unreferenced files", "session", sessionID, "files", n)
	}
}

// ImageStoreForTest exposes the store to the external test package, so the
// guarantees that the message read path would otherwise mask (it falls back to
// the original on any mismatch) can be tested head on.
type ImageStoreForTest struct{ st *imageStore }

// NewImageStoreForTest opens a store rooted at dir.
func NewImageStoreForTest(dir string) *ImageStoreForTest {
	return &ImageStoreForTest{&imageStore{dir: dir}}
}

func (x *ImageStoreForTest) Put(b []byte) (string, error)   { return x.st.put(b) }
func (x *ImageStoreForTest) Get(ref string) ([]byte, error) { return x.st.get(ref) }
func (x *ImageStoreForTest) Sweep(used map[string]bool, grace time.Duration) (int, error) {
	return x.st.sweep(used, grace)
}
