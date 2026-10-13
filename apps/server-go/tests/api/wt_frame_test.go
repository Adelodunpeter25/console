// WebTransport framing coverage: round-trip, truncated streams, and the
// length-prefix bound against corrupt peers.
package tests

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func TestWtFrameRoundTrip(t *testing.T) {
	frame := &consolev1.WtFrame{Seq: 7, Type: "ports", Payload: []byte(`{"ports":[]}`)}
	encoded, err := wt.EncodeFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint32(encoded[:4]); got != uint32(len(encoded)-4) {
		t.Fatalf("prefix %d != body %d", got, len(encoded)-4)
	}
	decoded, err := wt.DecodeFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.GetSeq() != 7 || decoded.GetType() != "ports" || string(decoded.GetPayload()) != `{"ports":[]}` {
		t.Fatalf("round-trip: %+v", decoded)
	}
}

func TestWtFrameBinaryPayloadSurvives(t *testing.T) {
	// Terminal/tunnel streams carry raw binary (existing tag-byte framing),
	// not protojson: arbitrary bytes must round-trip untouched.
	raw := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	encoded, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "output", Payload: raw})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := wt.DecodeFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.GetPayload(), raw) {
		t.Fatalf("binary payload corrupted: %x", decoded.GetPayload())
	}
}

func TestWtFrameTruncatedStreamFails(t *testing.T) {
	encoded, err := wt.EncodeFrame(&consolev1.WtFrame{Seq: 1, Type: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.DecodeFrame(bytes.NewReader(encoded[:len(encoded)-2])); err == nil {
		t.Fatal("truncated body must fail")
	}
	if _, err := wt.DecodeFrame(bytes.NewReader(encoded[:2])); err == nil {
		t.Fatal("truncated prefix must fail")
	}
	if _, err := wt.DecodeFrame(bytes.NewReader(nil)); err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatalf("empty stream must fail cleanly, got %v", err)
	}
}

func TestWtFrameOversizePrefixRejected(t *testing.T) {
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], wt.MaxFrameBytes+1)
	if _, err := wt.DecodeFrame(bytes.NewReader(prefix[:])); err == nil {
		t.Fatal("oversize prefix must fail without allocating")
	}
	var zero [4]byte
	if _, err := wt.DecodeFrame(bytes.NewReader(zero[:])); err == nil {
		t.Fatal("zero-length frame must fail")
	}
}

func TestWtFrameEncodeNilFails(t *testing.T) {
	if _, err := wt.EncodeFrame(nil); err == nil {
		t.Fatal("nil frame must fail")
	}
}

func TestWtStreamHeaderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := wt.WriteStreamHeader(&buf, consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN, "s1"); err != nil {
		t.Fatal(err)
	}
	kind, id, err := wt.ReadStreamHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if kind != consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN || id != "s1" {
		t.Fatalf("header = %v %q", kind, id)
	}
}

// The header reader must not over-consume: a header and its frame often
// arrive in one packet, and buffering past the header strands the frame in
// a discarded buffer (this stalled every stream once).
func TestWtStreamHeaderLeavesFrameBytes(t *testing.T) {
	var buf bytes.Buffer
	if err := wt.WriteStreamHeader(&buf, consolev1.WtStreamKind_WT_STREAM_KIND_TERMINAL, ""); err != nil {
		t.Fatal(err)
	}
	frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "spawn", Payload: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(frame)
	kind, id, err := wt.ReadStreamHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if kind != consolev1.WtStreamKind_WT_STREAM_KIND_TERMINAL || id != "" {
		t.Fatalf("header = %v %q", kind, id)
	}
	decoded, err := wt.DecodeFrame(&buf)
	if err != nil {
		t.Fatalf("frame after header: %v", err)
	}
	if decoded.GetType() != "spawn" {
		t.Fatalf("frame type = %q", decoded.GetType())
	}
}

func TestWtStreamHeaderRejectsBadKind(t *testing.T) {
	var buf bytes.Buffer
	// Unspecified kind.
	buf.Write([]byte{0x00, 0x00})
	if _, _, err := wt.ReadStreamHeader(&buf); err == nil {
		t.Fatal("unspecified kind must fail")
	}
	// Beyond the dispatched range.
	buf.Reset()
	buf.Write([]byte{0x40, 0x06, 0x00})
	if _, _, err := wt.ReadStreamHeader(&buf); err == nil {
		t.Fatalf("unknown kind must fail")
	}
}

func TestWtStreamHeaderRejectsOversizeID(t *testing.T) {
	if err := wt.WriteStreamHeader(io.Discard, consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN, strings.Repeat("x", 513)); err == nil {
		t.Fatal("oversize id must fail on write")
	}
}
