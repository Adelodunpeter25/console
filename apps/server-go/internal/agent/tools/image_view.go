// Image inspection for read_file: magic-byte sniffing, text/binary
// classification, and the inline image part shape shared with the browser,
// MCP, and CUA tools.
package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// SupportedImageMIMEs are the formats read_file returns as image parts.
// Anything sniffed but absent here (BMP, AVIF, PDF) stays on the text/binary
// path.
var SupportedImageMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// readMaxImageBytes is the ingest ceiling for a supported image, checked
// before the payload is read.
const readMaxImageBytes = 10 * 1024 * 1024

// ImageDescriber renders an image to prose for a run model that cannot see
// one. ok=false means no vision path is available and the caller must fall
// back to a placeholder — never a silent omission.
type ImageDescriber interface {
	// Describe returns a concise visual description. hint carries the file
	// path so the description can refer to what was read.
	Describe(ctx context.Context, mime string, data []byte, hint string) (description string, ok bool)
}

// ReadFileOptions tunes the read_file tool. The zero value is the static
// default used by DefaultTools().
type ReadFileOptions struct {
	// CanSeeImages is true when the image part will actually reach the run
	// model — the model declares image support *and* the provider delivers
	// images inside a tool result. When true, Describe is unused.
	CanSeeImages bool
	// Describer describes an image for a model that cannot see it; nil falls
	// back to a placeholder.
	Describer ImageDescriber
	// MaxImageBytes overrides readMaxImageBytes.
	MaxImageBytes int
	// RunModel is the active run model id and VisionModel the resolved vision
	// role ("provider/model"), both used only to label the handoff text.
	RunModel    string
	VisionModel string
}

// sniffImageMime identifies a supported image by magic bytes only — never by
// extension. Returns "" when the prefix is not a supported image.
func sniffImageMime(prefix []byte) string {
	switch {
	case bytes.HasPrefix(prefix, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case bytes.HasPrefix(prefix, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(prefix, []byte{0x47, 0x49, 0x46, 0x38}):
		return "image/gif"
	case len(prefix) >= 12 && bytes.HasPrefix(prefix, []byte("RIFF")) && bytes.Equal(prefix[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// imagePart renders raw image bytes as the inline content part every provider
// adapter understands.
func imagePart(mime string, data []byte) map[string]any {
	return map[string]any{
		"type":     "image",
		"data":     base64.StdEncoding.EncodeToString(data),
		"mimeType": mime,
	}
}

// binaryExtensions are known-binary, non-image suffixes rejected without
// reading further. Image suffixes are deliberately absent: images are decided
// by magic bytes, so a text file named ".png" must still read as text.
var binaryExtensions = map[string]bool{
	".pdf": true, ".zip": true, ".gz": true, ".tgz": true, ".tar": true,
	".bz2": true, ".xz": true, ".zst": true, ".7z": true, ".rar": true,
	".jar": true, ".war": true, ".class": true, ".pyc": true, ".pyo": true,
	".o": true, ".obj": true, ".a": true, ".lib": true, ".so": true,
	".dylib": true, ".dll": true, ".exe": true, ".bin": true, ".wasm": true,
	".sqlite": true, ".sqlite3": true, ".db": true, ".mdb": true, ".pack": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".m4a": true, ".wav": true, ".flac": true, ".ogg": true,
	".oga": true, ".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
	".webm": true, ".dmg": true, ".iso": true, ".img": true,
}

// isBinaryFile reports whether a non-image file should be refused instead of
// rendered as line-numbered text. A known-binary extension is decisive; on
// content, a NUL byte or invalid UTF-8 is decisive, and more than 30% control
// bytes marks binary. Ported from OpenCode's content heuristics.
func isBinaryFile(path string, prefix []byte) bool {
	if binaryExtensions[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	if len(prefix) == 0 {
		return false
	}
	if bytes.IndexByte(prefix, 0) >= 0 {
		return true
	}
	if !utf8ValidPrefix(prefix) {
		return true
	}
	controls := 0
	for _, b := range prefix {
		if b < 9 || (b > 13 && b < 32) {
			controls++
		}
	}
	return float64(controls)/float64(len(prefix)) > 0.3
}

// utf8ValidPrefix is utf8.Valid, tolerating up to three trailing bytes of a
// rune cut by the read boundary.
func utf8ValidPrefix(b []byte) bool {
	if utf8.Valid(b) {
		return true
	}
	for i := 1; i <= 3 && i < len(b); i++ {
		if utf8.Valid(b[:len(b)-i]) {
			return true
		}
	}
	return false
}
