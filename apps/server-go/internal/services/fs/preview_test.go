// Classification and media-metadata coverage for the unified file preview.
package fs

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewKindForPath(t *testing.T) {
	cases := map[string]PreviewKind{
		"a.png":     PreviewKindImage,
		"a.svg":     PreviewKindImage,
		"clip.mp4":  PreviewKindVideo,
		"clip.MOV":  PreviewKindVideo,
		"song.mp3":  PreviewKindAudio,
		"song.m4a":  PreviewKindAudio,
		"doc.pdf":   PreviewKindPdf,
		"arc.zip":   PreviewKindDetails,
		"font.ttf":  PreviewKindDetails,
		"app.exe":   PreviewKindDetails,
		"data.db":   PreviewKindDetails,
		"pic.tiff":  PreviewKindDetails,
		"main.go":   PreviewKindText,
		"readme.md": PreviewKindText,
		"Makefile":  PreviewKindText,
	}
	for name, want := range cases {
		if got := PreviewKindForPath(name); got != want {
			t.Errorf("PreviewKindForPath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCheckPreviewGateMediaNotBinary(t *testing.T) {
	// Video/audio/pdf must not hit BINARY_FILE anymore: they are served by
	// the media endpoint and details card, not text reads.
	for _, name := range []string{"clip.mp4", "song.mp3", "doc.pdf"} {
		if blocked := checkPreviewGate(name, 1024); blocked != nil {
			t.Errorf("%s should pass the gate, got %v", name, blocked.Code)
		}
	}
	// Opaque formats still dead-end as BINARY_FILE.
	for _, name := range []string{"arc.zip", "font.ttf", "data.db"} {
		blocked := checkPreviewGate(name, 1024)
		if blocked == nil || blocked.Code != "BINARY_FILE" {
			t.Errorf("%s should be BINARY_FILE, got %v", name, blocked)
		}
	}
	// Lockfiles stay gated.
	if blocked := checkPreviewGate("pnpm-lock.yaml", 10); blocked == nil || blocked.Code != "LOCKFILE_BLOCKED" {
		t.Errorf("lockfile should be LOCKFILE_BLOCKED, got %v", blocked)
	}
}

func TestGetMediaMeta(t *testing.T) {
	root := t.TempDir()
	svc := NewFsService()

	write := func(name string, data []byte) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// A 1x1 PNG carries decodable dimensions even through a .mp4 name? No —
	// dimensions are only attempted for image kinds; mp4 gets size/mtime only.
	mp4 := write("clip.mp4", []byte("fake-mp4-bytes"))
	meta, err := svc.GetMediaMeta(mp4)
	if err != nil {
		t.Fatalf("GetMediaMeta(mp4): %v", err)
	}
	if meta.Kind != PreviewKindVideo {
		t.Errorf("kind = %q, want video", meta.Kind)
	}
	if meta.MimeType != "video/mp4" {
		t.Errorf("mime = %q, want video/mp4", meta.MimeType)
	}
	if meta.SizeBytes != int64(len("fake-mp4-bytes")) {
		t.Errorf("size = %d", meta.SizeBytes)
	}
	if meta.MtimeMs <= 0 {
		t.Error("mtime should be populated")
	}

	pdf := write("doc.pdf", []byte("%PDF-1.4 fake"))
	meta, err = svc.GetMediaMeta(pdf)
	if err != nil {
		t.Fatalf("GetMediaMeta(pdf): %v", err)
	}
	if meta.Kind != PreviewKindPdf || meta.MimeType != "application/pdf" {
		t.Errorf("pdf meta = %+v", meta)
	}

	// Non-media kinds are rejected.
	for _, name := range []string{"main.go", "arc.zip", "a.png"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.GetMediaMeta(path); err == nil {
			t.Errorf("GetMediaMeta(%s) should fail", name)
		}
	}

	// Directories are rejected.
	if _, err := svc.GetMediaMeta(root); err == nil {
		t.Error("GetMediaMeta(dir) should fail")
	}
}

func TestGetMediaMetaImageDimensions(t *testing.T) {
	// Verify the dimension helper against a real PNG.
	path := filepath.Join(t.TempDir(), "dot.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	w, h, ok := imageDimensions(path)
	if !ok || w != 2 || h != 3 {
		t.Errorf("imageDimensions = %d x %d ok=%v, want 2x3", w, h, ok)
	}
}
