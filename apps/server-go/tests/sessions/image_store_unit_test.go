// The image store, tested directly. The message read path falls back to the
// original content whenever a rebuilt message differs, which is right for users
// and wrong for a test of a safety rule: a read of a forbidden file that happens
// to produce different bytes would be silently discarded and the test would pass.
// These tests call the store itself, where nothing can hide.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
)

func TestStoreGetRefusesAnythingThatIsNotADigest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "images")
	st := session.NewImageStoreForTest(dir)
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, ref := range map[string]string{
		"traversal":    "../secret.txt",
		"deep":         "../../../../../../../../etc/hosts",
		"absolute":     secret,
		"empty":        "",
		"short":        strings.Repeat("a", 63),
		"long":         strings.Repeat("a", 65),
		"uppercase":    strings.Repeat("A", 64),
		"non hex":      strings.Repeat("g", 64),
		"embedded dot": strings.Repeat("a", 31) + "/../" + strings.Repeat("b", 29),
		"nul":          strings.Repeat("a", 63) + "\x00",
	} {
		if b, err := st.Get(ref); err == nil {
			t.Errorf("%s: Get accepted %q and returned %d bytes", name, ref, len(b))
		}
	}
}

func TestStoreGetRefusesASymlinkAndAFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "images")
	st := session.NewImageStoreForTest(dir)
	ref, err := st.Put([]byte("real image bytes"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ref)
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("VICTIM CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Skip("symlinks unavailable")
	}
	if b, err := st.Get(ref); err == nil {
		t.Fatalf("a symlink was followed and returned %q", b)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ref); err == nil {
		t.Fatal("a directory was treated as an image")
	}
}

func TestStorePutThenGetRoundTripsAndIsContentAddressed(t *testing.T) {
	st := session.NewImageStoreForTest(filepath.Join(t.TempDir(), "images"))
	a, err := st.Put([]byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := st.Put([]byte("one"))
	c, _ := st.Put([]byte("two"))
	if a != b || a == c {
		t.Fatalf("same bytes must give the same ref and different bytes a different one: %s %s %s", a, b, c)
	}
	if got, err := st.Get(a); err != nil || string(got) != "one" {
		t.Fatalf("get: %q %v", got, err)
	}
}

func TestStoreSweepOnlyRemovesOldUnreferencedFilesOfItsOwn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	st := session.NewImageStoreForTest(dir)
	keep, _ := st.Put([]byte("referenced"))
	oldGone, _ := st.Put([]byte("unreferenced and old"))
	freshGone, _ := st.Put([]byte("unreferenced but fresh"))
	foreign := filepath.Join(dir, "notes.txt")
	staleTmp := filepath.Join(dir, ".tmp-leftover")
	for _, p := range []string{foreign, staleTmp} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-3 * time.Hour)
	for _, p := range []string{filepath.Join(dir, keep), filepath.Join(dir, oldGone), foreign, staleTmp} {
		_ = os.Chtimes(p, old, old)
	}

	removed, err := st.Sweep(map[string]bool{keep: true}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool { _, err := os.Lstat(filepath.Join(dir, name)); return err == nil }
	if !exists(keep) {
		t.Error("a referenced image was deleted")
	}
	if exists(oldGone) {
		t.Error("an old unreferenced image was not removed")
	}
	if !exists(filepath.Base(filepath.Join(dir, freshGone))) {
		t.Error("a fresh unreferenced image was removed inside the grace period")
	}
	if !exists("notes.txt") {
		t.Error("a file the store did not create was removed")
	}
	if exists(".tmp-leftover") {
		t.Error("a stale temp file from an interrupted write was not cleaned up")
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2 (the old image and the stale temp file)", removed)
	}
}

func TestStoreSweepNeverDeletesFoldersOrSymlinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "images")
	st := session.NewImageStoreForTest(dir)
	if _, err := st.Put([]byte("x")); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, "victim.txt")
	_ = os.WriteFile(victim, []byte("keep me"), 0o644)
	digestLike := strings.Repeat("c", 64)
	if err := os.Symlink(victim, filepath.Join(dir, digestLike)); err != nil {
		t.Skip("symlinks unavailable")
	}
	folderLike := strings.Repeat("d", 64)
	_ = os.Mkdir(filepath.Join(dir, folderLike), 0o755)
	_ = os.WriteFile(filepath.Join(dir, folderLike, "inner"), []byte("inner"), 0o644)
	old := time.Now().Add(-5 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, folderLike), old, old)

	if _, err := st.Sweep(map[string]bool{}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "keep me" {
		t.Fatal("the sweep reached through a symlink and damaged the file it pointed at")
	}
	// os.Remove on a link removes the link itself, which the store never made,
	// so the link must still be there, as must an empty folder with a digest name.
	if info, err := os.Lstat(filepath.Join(dir, digestLike)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the sweep removed a symlink it did not create")
	}
	emptyFolder := strings.Repeat("e", 64)
	if err := os.Mkdir(filepath.Join(dir, emptyFolder), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(filepath.Join(dir, emptyFolder), old, old)
	if _, err := st.Sweep(map[string]bool{}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(dir, emptyFolder)); err != nil || !info.IsDir() {
		t.Fatal("the sweep removed an empty directory")
	}
	if _, err := os.Stat(filepath.Join(dir, folderLike, "inner")); err != nil {
		t.Fatal("the sweep removed a directory")
	}
}
