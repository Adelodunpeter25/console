package services

import "unicode/utf8"

// utf8Decoder joins partial multibyte sequences split across pipe chunks,
// matching Bun's TextDecoder({stream:true}) behavior.
type utf8Decoder struct {
	pending []byte
}

func newUTF8Decoder() *utf8Decoder {
	return &utf8Decoder{}
}

// UTF8Decoder is the exported streaming decoder shared by the PTY terminal
// path (JSON mode) and bash capture. It buffers a trailing partial rune
// across chunks so multibyte characters split across reads survive.
type UTF8Decoder = utf8Decoder

// NewUTF8Decoder creates a streaming UTF-8 decoder.
func NewUTF8Decoder() *UTF8Decoder {
	return newUTF8Decoder()
}

// Decode decodes chunk, holding back a trailing partial rune for the next
// call. Invalid bytes are replaced downstream by encoding/json.
func (d *UTF8Decoder) Decode(chunk []byte) string {
	return d.decode(chunk)
}

func (d *utf8Decoder) decode(chunk []byte) string {
	data := append(d.pending, chunk...)
	d.pending = nil
	if utf8.Valid(data) {
		return string(data)
	}
	// Find the longest valid prefix; keep the trailing partial rune.
	cut := len(data)
	for cut > 0 && cut > len(data)-utf8.UTFMax {
		if r, _ := utf8.DecodeLastRune(data[:cut]); r != utf8.RuneError {
			break
		}
		cut--
	}
	if cut < len(data) {
		d.pending = append(d.pending, data[cut:]...)
		data = data[:cut]
	}
	return string(data)
}
