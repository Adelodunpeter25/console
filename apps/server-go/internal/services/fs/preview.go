// File preview gating, formatting, and file-type utilities.
package fs

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxFilePreviewBytes = 10 * 1024 * 1024
	imageMaxBytes       = 10 * 1024 * 1024
)

// PreviewBlocked carries the structured code/status the /file routes emit.
type PreviewBlocked struct {
	Code   string // LOCKFILE_BLOCKED | BINARY_FILE | FILE_TOO_LARGE
	Status int
	Detail map[string]any
	Msg    string
}

func (e *PreviewBlocked) Error() string { return e.Msg }

func BuildFileETag(sizeBytes int64, mtimeMs int64) string {
	return fmt.Sprintf("W/\"%s-%s\"",
		strconv.FormatInt(sizeBytes, 36),
		strconv.FormatInt(mtimeMs, 36))
}

var lockFileBasenames = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true,
	"pnpm-lock.yaml": true, "composer.lock": true,
}

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
	".ico": true, ".icns": true, ".tiff": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true, ".jar": true, ".war": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true, ".o": true,
	".obj": true, ".bin": true, ".iso": true, ".dmg": true, ".pkg": true, ".deb": true,
	".rpm": true, ".apk": true, ".ipa": true, ".node": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".pdf": true, ".wasm": true, ".psd": true, ".sketch": true, ".class": true,
	".pyc": true, ".db": true, ".sqlite": true, ".sqlite3": true,
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".svg": true,
}

// checkPreviewGate returns a *PreviewBlocked when the file cannot be shown.
func checkPreviewGate(path string, size int64) *PreviewBlocked {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	if lockFileBasenames[base] || ext == ".lock" || ext == ".lockb" {
		return &PreviewBlocked{Code: "LOCKFILE_BLOCKED", Status: 403,
			Msg: fmt.Sprintf("Machine-generated lockfile %q cannot be previewed.", base)}
	}
	if binaryExtensions[ext] {
		return &PreviewBlocked{Code: "BINARY_FILE", Status: 403,
			Msg: fmt.Sprintf("Binary file %q cannot be previewed as text.", base)}
	}
	if size > maxFilePreviewBytes {
		return &PreviewBlocked{Code: "FILE_TOO_LARGE", Status: 413,
			Msg:    fmt.Sprintf("%q is %s — previews are capped at %s.", base, formatBytes(size), formatBytes(maxFilePreviewBytes)),
			Detail: map[string]any{"sizeBytes": size, "maxBytes": maxFilePreviewBytes}}
	}
	return nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
