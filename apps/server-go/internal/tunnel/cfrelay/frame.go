package cfrelay

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Frame types of the relay protocol (see apps/relay/README.md). Every
// message is binary: [type u8][stream id u32 big-endian][payload].
const (
	frameReq    byte = 1
	frameData   byte = 2
	frameEnd    byte = 3
	frameRes    byte = 4
	frameWSOpen byte = 5
	frameWSText byte = 6
	frameWSBin  byte = 7
	frameClose  byte = 8
	frameReset  byte = 9
)

const (
	headerLen = 5
	// chunkSize keeps every message well under the relay's 1 MiB limit.
	chunkSize = 256 * 1024
)

var errShortFrame = errors.New("cfrelay: short frame")

func encodeFrame(kind byte, sid uint32, payload []byte) []byte {
	out := make([]byte, headerLen+len(payload))
	out[0] = kind
	binary.BigEndian.PutUint32(out[1:], sid)
	copy(out[headerLen:], payload)
	return out
}

func decodeFrame(msg []byte) (kind byte, sid uint32, payload []byte, err error) {
	if len(msg) < headerLen {
		return 0, 0, nil, errShortFrame
	}
	return msg[0], binary.BigEndian.Uint32(msg[1:]), msg[headerLen:], nil
}

type requestHead struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Headers [][2]string `json:"headers"`
}

type responseHead struct {
	Status  int         `json:"status"`
	Headers [][2]string `json:"headers"`
}

type closeBody struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

func jsonBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func parseLen(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, err == nil && n >= 0
}
