// Image fitting: pass-through for small images, downscale/re-encode over the
// budget, and a clear failure for an oversized format the stdlib cannot decode.
package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	imageutil "github.com/Adelodunpeter25/console/apps/server-go/internal/utils/image"
)

func solidPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 30, G: 90, B: 200, A: 255}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func noisyJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(1))
	for i := range img.Pix {
		img.Pix[i] = byte(rnd.Intn(256))
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFitImage(t *testing.T) {
	// A small PNG passes through byte-identical.
	small := solidPNG(t, 8, 8)
	p, err := imageutil.Fit(small, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if p.Resized || !bytes.Equal(p.Data, small) || p.Mime != "image/png" {
		t.Fatalf("small image must pass through: resized=%v mime=%s", p.Resized, p.Mime)
	}
	if p.Width != 8 || p.Height != 8 {
		t.Fatalf("dimensions: %dx%d", p.Width, p.Height)
	}

	// An oversized PNG is downscaled to the long-edge budget.
	big := solidPNG(t, 2500, 1500)
	p, err = imageutil.Fit(big, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Resized || p.Mime != "image/png" {
		t.Fatalf("oversized PNG: resized=%v mime=%s", p.Resized, p.Mime)
	}
	if p.Width != imageutil.MaxEdge || p.Height != 1200 {
		t.Fatalf("scaled dimensions: %dx%d", p.Width, p.Height)
	}
	if imageutil.MaxBase64 < base64.StdEncoding.EncodedLen(len(p.Data)) {
		t.Fatal("fitted payload over budget")
	}

	// A photographic JPEG over the raw budget is re-encoded as JPEG.
	photo := noisyJPEG(t, 2400, 1600)
	p, err = imageutil.Fit(photo, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if p.Mime != "image/jpeg" {
		t.Fatalf("photographic image must stay JPEG: %s", p.Mime)
	}
	if imageutil.MaxBase64 < base64.StdEncoding.EncodedLen(len(p.Data)) {
		t.Fatal("fitted photo over budget")
	}

	// An oversized WebP cannot be decoded by the stdlib and is refused.
	webp := make([]byte, 4*1024*1024)
	copy(webp, []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'})
	if _, err := imageutil.Fit(webp, "image/webp"); err == nil {
		t.Fatal("oversized WebP must fail")
	}
}

func TestReadFileResizesImage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.png")
	if err := os.WriteFile(p, solidPNG(t, 2500, 1500), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"path": p})
	out, err := tools.ReadFile.Execute(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	parts := out.([]map[string]any)
	if parts[0]["mimeType"] != "image/png" {
		t.Fatalf("fitted mime must propagate: %#v", parts[0])
	}
	text, _ := parts[1]["text"].(string)
	if !strings.Contains(text, "2000x1200") {
		t.Fatalf("fitted dimensions must propagate: %q", text)
	}
	data, _ := base64.StdEncoding.DecodeString(parts[0]["data"].(string))
	if imageutil.MaxBase64 < base64.StdEncoding.EncodedLen(len(data)) {
		t.Fatal("emitted image over budget")
	}
}
