// Package hostc speaks the hostc tunnel protocol so a local server can be
// reached through a public HTTPS URL.
//
// This is a native Go implementation of the client half of the protocol
// (version 5), described at https://github.com/akazwz/hostc. It exists so a
// server on a host with no dialable address — a cloud VM behind NAT, a load
// balancer, a locked-down network — can still be reached, without needing a
// Node runtime on that host.
//
// One Client owns one tunnel: a single WebSocket to the tunnel server, with
// every public request and WebSocket multiplexed over it.
package hostc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	// ProtocolVersion is the wire version this client speaks. The tunnel
	// server rejects a mismatch outright (426); there is no negotiation.
	ProtocolVersion = 5

	// ProtocolHeader carries ProtocolVersion on tunnel API calls.
	ProtocolHeader = "hostc-protocol"

	// Ping and Pong are the only text messages on the tunnel connection.
	// Everything else is a binary frame.
	Ping = "hostc:ping"
	Pong = "hostc:pong"

	// HeartbeatInterval is how often the client pings the server.
	HeartbeatInterval = 15 * time.Second
	// HeartbeatTimeout is how long the client waits for a pong before
	// treating a silently dead connection as lost.
	HeartbeatTimeout = 10 * time.Second

	// MaxChunkBytes is the largest DATA payload for an HTTP body chunk.
	MaxChunkBytes = 64 * 1024

	// StreamWindowBytes is the per-stream, per-direction flow control window.
	StreamWindowBytes = 256 * 1024

	// MaxWebSocketMessageBytes is the largest WebSocket message forwarded.
	MaxWebSocketMessageBytes = 1024*1024 - 64

	// CloseShutdown releases the tunnel immediately.
	CloseShutdown = 4000
	// CloseProtocolError signals a protocol violation.
	CloseProtocolError = 1002
)

// Frame types on the tunnel wire.
const (
	FrameOpen   byte = 1
	FrameHead   byte = 2
	FrameData   byte = 3
	FrameText   byte = 4
	FrameEnd    byte = 5
	FrameReset  byte = 6
	FrameWindow byte = 7
)

// frameHeaderBytes is the frame header size: u8 type + u32 stream id.
const frameHeaderBytes = 5

// ErrProtocol marks a violation of the tunnel protocol. The connection is
// closed rather than a single stream being dropped.
var ErrProtocol = errors.New("hostc: protocol error")

// Frame is one message on the tunnel connection.
type Frame struct {
	Type    byte
	Stream  uint32
	Payload []byte
}

// Encode renders the frame for the wire.
func (f Frame) Encode() []byte {
	out := make([]byte, frameHeaderBytes+len(f.Payload))
	out[0] = f.Type
	binary.BigEndian.PutUint32(out[1:frameHeaderBytes], f.Stream)
	copy(out[frameHeaderBytes:], f.Payload)
	return out
}

// DecodeFrame parses a frame from the wire.
func DecodeFrame(b []byte) (Frame, error) {
	if len(b) < frameHeaderBytes {
		return Frame{}, fmt.Errorf("%w: frame too short (%d bytes)", ErrProtocol, len(b))
	}
	if b[0] < FrameOpen || b[0] > FrameWindow {
		return Frame{}, fmt.Errorf("%w: unknown frame type %d", ErrProtocol, b[0])
	}
	id := binary.BigEndian.Uint32(b[1:frameHeaderBytes])
	if id == 0 {
		return Frame{}, fmt.Errorf("%w: stream id 0 is reserved", ErrProtocol)
	}
	return Frame{Type: b[0], Stream: id, Payload: b[frameHeaderBytes:]}, nil
}

// HeaderList is a header set carried as name/value pairs, matching the wire
// format (a list of two-element arrays).
type HeaderList [][2]string

// OpenMessage starts a stream: the tunnel server asking the client to make
// a request against the local target.
type OpenMessage struct {
	Method  string     `json:"method"`
	Path    string     `json:"path"`
	Headers HeaderList `json:"headers"`
	Body    bool       `json:"body"`
	// WebSocket marks the stream as a WebSocket upgrade and lists the
	// subprotocols the public client offered.
	//
	// The key's presence is what marks the upgrade, not its length: a
	// public client that offers no subprotocol sends an empty list, and
	// that is still a WebSocket. A nil pointer means the key was absent,
	// which means a plain HTTP request.
	WebSocket *[]string `json:"websocket,omitempty"`
}

// HeadMessage answers an OpenMessage with the local response's status and
// headers.
type HeadMessage struct {
	Status  int        `json:"status"`
	Headers HeaderList `json:"headers"`
	Body    bool       `json:"body"`
	// Protocol is the subprotocol the local WebSocket server selected. It
	// is set only for a 101 response.
	Protocol string `json:"protocol,omitempty"`
}

// CloseMessage is the END payload of a WebSocket stream: a u16 close code
// followed by a UTF-8 reason.
type CloseMessage struct {
	Code   uint16
	Reason string
}

// encodeClose renders a close message.
func encodeClose(m CloseMessage) []byte {
	reason := []byte(m.Reason)
	out := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(out[:2], m.Code)
	copy(out[2:], reason)
	return out
}

// decodeClose parses a close message.
func decodeClose(b []byte) CloseMessage {
	if len(b) < 2 {
		return CloseMessage{Code: 1005}
	}
	return CloseMessage{
		Code:   binary.BigEndian.Uint16(b[:2]),
		Reason: string(b[2:]),
	}
}

// EncodeWindow renders a WINDOW payload: the bytes granted to the peer.
func EncodeWindow(n uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, n)
	return out
}

// decodeWindow parses a WINDOW payload.
func decodeWindow(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, fmt.Errorf("%w: WINDOW payload must be 4 bytes", ErrProtocol)
	}
	n := binary.BigEndian.Uint32(b)
	if n == 0 {
		return 0, fmt.Errorf("%w: empty WINDOW grant", ErrProtocol)
	}
	return n, nil
}

// decodeOpen parses an OPEN payload.
func decodeOpen(b []byte) (OpenMessage, error) {
	var m OpenMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return OpenMessage{}, fmt.Errorf("%w: invalid OPEN payload: %v", ErrProtocol, err)
	}
	if m.Method == "" || len(m.Path) == 0 || m.Path[0] != '/' {
		return OpenMessage{}, fmt.Errorf("%w: invalid OPEN payload", ErrProtocol)
	}
	return m, nil
}

// decodeHead parses a HEAD payload.
func decodeHead(b []byte) (HeadMessage, error) {
	var m HeadMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return HeadMessage{}, fmt.Errorf("%w: invalid HEAD payload: %v", ErrProtocol, err)
	}
	if m.Status < 100 || m.Status > 599 {
		return HeadMessage{}, fmt.Errorf("%w: invalid HEAD status %d", ErrProtocol, m.Status)
	}
	return m, nil
}

// responseHasBody reports whether a response to method with status carries a
// body (RFC 9110 section 6.4.1).
func responseHasBody(method string, status int) bool {
	return method != "HEAD" && status >= 200 && status != 204 && status != 205 && status != 304
}
