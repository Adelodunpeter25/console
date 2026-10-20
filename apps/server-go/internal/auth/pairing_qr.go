// The pairing QR code.
//
// The other device scans this to redeem a one-time grant. It is rendered
// here, on the server, because the server is the one that knows the grant
// and the server id — and because it already has a QR encoder on hand for
// the terminal.
//
// The payload is the full pairing URL: the address the new device should
// talk to, and the code, in the URL's fragment. A phone that scans it
// fills in both fields itself, so nobody has to read a code out loud.
package auth

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// PairingPayload is the text a QR code for a pairing carries.
//
// The base is the address the *other* device should reach this server at.
// The caller knows it (it is the address it is talking to now), the server
// does not: it may be behind a proxy or a tunnel, and its own bind
// address is often a wildcard that means nothing to a phone.
func PairingPayload(baseURL, serverID, code string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "http://localhost:3000"
	}
	return PairingURL(trimmed, serverID, code)
}

// RenderPairingQR renders a pairing payload as a PNG.
//
// The image is drawn module by module at an exact pixel size, rather than
// scaled to fit: a scaled QR has modules of uneven width, and a scanner
// samples the middle of each module, so a module that is one pixel short
// can be read as the wrong value.
//
// Returns nil when the payload cannot be encoded — a QR is a convenience,
// and the code and the URL in the response are the actual means of
// pairing, so a failure here must not fail the request.
func RenderPairingQR(payload string) []byte {
	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return nil
	}
	// The bitmap already carries the quiet zone, which a scanner needs to
	// find the code's edges.
	bitmap := code.Bitmap()
	modules := len(bitmap)
	if modules == 0 {
		return nil
	}
	const modulePixels = 8
	size := modules * modulePixels
	img := image.NewGray(image.Rect(0, 0, size, size))
	// The image starts black, and a QR is dark modules on a light
	// background. Filling it white first is what makes the code readable
	// at all — an inverted symbol is not a QR any scanner will take.
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	for y := range modules {
		for x := range modules {
			if !bitmap[y][x] {
				continue
			}
			for dy := 0; dy < modulePixels; dy++ {
				for dx := 0; dx < modulePixels; dx++ {
					img.SetGray(x*modulePixels+dx, y*modulePixels+dy, color.Gray{Y: 0})
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
