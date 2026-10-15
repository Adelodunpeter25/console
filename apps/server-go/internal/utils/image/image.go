// Package imageutil sniffs, sizes, and re-encodes inline images so tool
// results stay within provider budgets. Pure Go: no cgo, no wasm.
package imageutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

const (
	// MaxEdge is the longest allowed side after resizing.
	MaxEdge = 2000
	// MaxBase64 is the budget for the base64-encoded payload.
	MaxBase64 = 5 * 1024 * 1024
	// maxRawBytes is the largest raw payload whose base64 encoding still fits
	// MaxBase64.
	maxRawBytes = MaxBase64 / 4 * 3
	jpegQuality = 85
	maxAttempts = 4
)

// SupportedMIMEs are the formats we emit as image parts. Anything sniffed but
// absent here stays on the text/binary path.
var SupportedMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// SniffMime identifies a supported image by magic bytes only — never by
// extension. Returns "" when the prefix is not a supported image.
func SniffMime(prefix []byte) string {
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

// Payload is a sniffed, size-checked image ready for passthrough or encode.
type Payload struct {
	Mime    string
	Data    []byte // the bytes that get base64'd
	Width   int
	Height  int
	Resized bool
}

// Fit returns raw unchanged when it already fits the budget, otherwise a
// downscaled re-encode. It fails only when the image cannot be decoded and the
// original is over budget (e.g. an oversized WebP the stdlib cannot decode).
func Fit(raw []byte, mime string) (Payload, error) {
	p := Payload{Mime: mime, Data: raw}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
		p.Width, p.Height = cfg.Width, cfg.Height
	}
	if len(raw) <= maxRawBytes && p.Width <= MaxEdge && p.Height <= MaxEdge {
		return p, nil
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		if len(raw) <= maxRawBytes {
			return Payload{Mime: mime, Data: raw}, nil
		}
		return Payload{}, fmt.Errorf("image is too large and its format cannot be resized")
	}

	w, h := p.Width, p.Height
	if w <= 0 || h <= 0 {
		b := img.Bounds()
		w, h = b.Dx(), b.Dy()
	}
	if w > MaxEdge || h > MaxEdge {
		scale := min(1.0, float64(MaxEdge)/float64(w), float64(MaxEdge)/float64(h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		scaled := scaleImage(img, w, h)
		if data, err := encodePNG(scaled); err == nil && base64Len(len(data)) <= MaxBase64 {
			return Payload{Mime: "image/png", Data: data, Width: w, Height: h, Resized: true}, nil
		}
		if data, err := encodeJPEG(scaled); err == nil && base64Len(len(data)) <= MaxBase64 {
			return Payload{Mime: "image/jpeg", Data: data, Width: w, Height: h, Resized: true}, nil
		}
		w, h = w/2, h/2
	}
	return Payload{}, fmt.Errorf("image is too large to fit the %d byte base64 budget", MaxBase64)
}

func scaleImage(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func base64Len(n int) int { return base64.StdEncoding.EncodedLen(n) }
