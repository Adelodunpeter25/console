// WebTransport framing (QUIC migration, Phase 1): one length-prefixed
// frame is [u32BE len][protojson(WtFrame)]. DecodeFrame bounds the length
// prefix so a corrupt peer cannot make the server allocate unbounded memory.
package wt

import (
	"encoding/binary"
	"fmt"
	"io"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// MaxFrameBytes caps a single decoded frame (terminal bursts included).
// Larger than any legitimate control frame by orders of magnitude.
const MaxFrameBytes = 8 * 1024 * 1024

var protoJSON = protojson.MarshalOptions{}

// EncodeFrame serializes one frame for a WT stream body.
func EncodeFrame(frame *consolev1.WtFrame) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("cannot encode a nil WT frame.")
	}
	raw, err := protoJSON.Marshal(frame)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxFrameBytes {
		return nil, fmt.Errorf("WT frame of %d bytes exceeds the %d-byte cap.", len(raw), MaxFrameBytes)
	}
	out := make([]byte, 4, 4+len(raw))
	binary.BigEndian.PutUint32(out, uint32(len(raw)))
	return append(out, raw...), nil
}

// DecodeFrame reads one length-prefixed frame from a stream.
func DecodeFrame(r io.Reader) (*consolev1.WtFrame, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(prefix[:])
	if size == 0 || size > MaxFrameBytes {
		return nil, fmt.Errorf("WT frame length %d out of bounds.", size)
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, err
	}
	var frame consolev1.WtFrame
	if err := protojson.Unmarshal(raw, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}
