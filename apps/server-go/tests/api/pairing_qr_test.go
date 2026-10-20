// Pairing QR coverage: the rendered image must be exactly the modules the
// encoder produced, dark on a light background, with the quiet zone intact.
//
// The failure this guards against is subtle and silent: an image that is
// inverted, or drawn from the wrong data, still *looks* like a QR to a
// person and fails only when a phone tries to scan it.
package tests

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
)

func TestPairingQRDrawsTheEncodedModules(t *testing.T) {
	payload := auth.PairingPayload("http://192.168.1.10:3000", "server-1", "abc-123-xyz")
	raw := auth.RenderPairingQR(payload)
	if len(raw) == 0 {
		t.Fatal("expected a rendered QR code")
	}

	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	img, ok := decoded.(*image.Gray)
	if !ok {
		t.Fatalf("expected a grayscale image, got %T", decoded)
	}

	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	bitmap := code.Bitmap()
	modules := len(bitmap)
	if modules == 0 {
		t.Fatal("empty symbol")
	}
	module := img.Bounds().Dx() / modules
	if module*modules != img.Bounds().Dx() {
		t.Fatalf("image width %d is not a whole number of %d modules", img.Bounds().Dx(), modules)
	}

	// Every module must match, sampled at its centre.
	for y := range modules {
		for x := range modules {
			pixel := img.GrayAt(x*module+module/2, y*module+module/2).Y
			dark := pixel < 128
			if dark != bitmap[y][x] {
				t.Fatalf("module (%d,%d): rendered dark=%v, encoded dark=%v", x, y, dark, bitmap[y][x])
			}
		}
	}

	// The quiet zone has to be light, or a scanner cannot find the code's
	// edges.
	for i := 0; i < 4*module; i++ {
		if img.GrayAt(i, i).Y < 128 {
			t.Fatal("the quiet zone must be light")
		}
	}
}

func TestPairingPayloadCarriesTheCode(t *testing.T) {
	payload := auth.PairingPayload("http://192.168.1.10:3000/", "server-1", "abc-123-xyz")
	if payload != "http://192.168.1.10:3000/console/pair#code=abc-123-xyz&server=server-1&v=1" {
		t.Fatalf("payload: %s", payload)
	}
	// The code lives in the fragment, so it never reaches the server in a
	// request.
	if idx := len("http://192.168.1.10:3000/console/pair#"); payload[idx:] == "" {
		t.Fatal("the payload has no code")
	}
}
