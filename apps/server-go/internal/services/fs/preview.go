// File preview gating, formatting, and file-type utilities.
package fs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/MoeForge/moego/regexp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxFilePreviewBytes = 10 * 1024 * 1024
	imageMaxBytes       = 10 * 1024 * 1024
	// mediaMaxBytes caps the /api/fs/file/media endpoint; media is streamed
	// with Range support so large videos stay seekable.
	mediaMaxBytes = 2 * 1024 * 1024 * 1024
	// mediaDurationProbeTimeout bounds the best-effort ffprobe call so a
	// missing or hung binary never stalls the metadata request.
	mediaDurationProbeTimeout = 5 * time.Second
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

// PreviewKind classifies a file for the unified preview experience. It is
// kept in sync with the desktop FileKind and Android FilePreviewKind lists.
type PreviewKind string

const (
	PreviewKindImage   PreviewKind = "image"
	PreviewKindVideo   PreviewKind = "video"
	PreviewKindAudio   PreviewKind = "audio"
	PreviewKindPdf     PreviewKind = "pdf"
	PreviewKindText    PreviewKind = "text"
	PreviewKindDetails PreviewKind = "details"
)

// Extension sets below are mirrored exactly in
// apps/desktop/crates/console-shared/src/file_kind.rs and
// apps/android/.../data/model/FsModels.kt — keep all three in sync.
var videoExtensions = map[string]bool{
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
}

var audioExtensions = map[string]bool{
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
}

var pdfExtensions = map[string]bool{
	".pdf": true,
}

// opaqueExtensions is the details-card set: formats with no inline viewer
// get a metadata card instead of the old binary dead-end.
var opaqueExtensions = map[string]bool{
	".icns": true, ".tiff": true,
	// Archives
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true, ".jar": true, ".war": true,
	// Executables / objects / disk images
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true, ".o": true,
	".obj": true, ".bin": true, ".iso": true, ".dmg": true, ".pkg": true, ".deb": true,
	".rpm": true, ".apk": true, ".ipa": true, ".node": true,
	// Fonts
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	// Documents / other opaque formats
	".wasm": true, ".psd": true, ".sketch": true, ".class": true,
	".pyc": true, ".db": true, ".sqlite": true, ".sqlite3": true,
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".svg": true,
}

// PreviewKindForPath classifies a file by extension. Lockfiles still report
// their own gate code at read time; callers should check the gate first.
func PreviewKindForPath(path string) PreviewKind {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case imageExtensions[ext]:
		return PreviewKindImage
	case videoExtensions[ext]:
		return PreviewKindVideo
	case audioExtensions[ext]:
		return PreviewKindAudio
	case pdfExtensions[ext]:
		return PreviewKindPdf
	case opaqueExtensions[ext]:
		return PreviewKindDetails
	default:
		return PreviewKindText
	}
}

// checkPreviewGate returns a *PreviewBlocked when the file cannot be shown.
func checkPreviewGate(path string, size int64) *PreviewBlocked {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	if lockFileBasenames[base] || ext == ".lock" || ext == ".lockb" {
		return &PreviewBlocked{Code: "LOCKFILE_BLOCKED", Status: 403,
			Msg: fmt.Sprintf("Machine-generated lockfile %q cannot be previewed.", base)}
	}
	// Only opaque formats stay gated: video/audio/PDF are served by the media
	// endpoint and details by the metadata card, so they are not "text" reads.
	if opaqueExtensions[ext] {
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

// MediaMeta describes a file servable by GET /api/fs/file/media. Optional
// fields are pointers so clients can distinguish "unknown" from zero.
type MediaMeta struct {
	Kind       PreviewKind `json:"kind"`
	MimeType   string      `json:"mimeType"`
	SizeBytes  int64       `json:"sizeBytes"`
	MtimeMs    int64       `json:"mtimeMs"`
	Width      *int        `json:"width,omitempty"`
	Height     *int        `json:"height,omitempty"`
	DurationMs *int64      `json:"durationMs,omitempty"`
}

// mediaMimeType maps the media extensions to a Content-Type. Falls back to
// mime.TypeByExtension so uncommon-but-valid types still get a sane label.
func mediaMimeType(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".pdf":
		return "application/pdf"
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// imageDimensions decodes just the header of an image file for its pixel
// size. Best-effort: returns false for formats the stdlib cannot sniff
// (webp, ico variants) or unreadable files.
func imageDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	head := make([]byte, 512*1024)
	n, _ := f.Read(head)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head[:n]))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

var ffprobeDurationRe = regexp.MustCompile(`"duration"\s*:\s*"([0-9.]+)"`)

// probeDurationMs asks ffprobe for the container duration in milliseconds.
// Best-effort: returns nil when ffprobe is absent, times out, or fails.
func probeDurationMs(path string) *int64 {
	ctx, cancel := context.WithTimeout(context.Background(), mediaDurationProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet", "-show_entries", "format=duration",
		"-of", "json", path).Output()
	if err != nil {
		return nil
	}
	m := ffprobeDurationRe.FindSubmatch(out)
	if m == nil {
		return nil
	}
	secs, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil || secs < 0 {
		return nil
	}
	ms := int64(secs * 1000)
	return &ms
}
