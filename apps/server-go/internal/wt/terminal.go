// TERMINAL streams: spawn a PTY on the open frame, then pump output and
// apply input until either side goes away — the WS lifecycle transposed
// onto one QUIC bidi stream.
//
// Server→client frames mirror the WS shapes exactly: spawned/exit/error
// (and text-mode output) as protojson TerminalServerMessage payloads,
// binary-mode output as raw tag-byte frames. Client→server frames are
// WtFrame envelopes: type "input" with raw PTY bytes, "resize"/"kill" with
// the matching protojson payloads. Resize/kill ride the stream in v1
// (unlike REST, there is no parallel unary channel inside a migration
// handshake, and racing a bare handshake cannot double-fire them —
// Phase 5 races connects, never streams).
package wt

import (
	"sync"

	webtransport "github.com/quic-go/webtransport-go"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

const (
	terminalOutputFrameTag = 0x01
	// Mirror the WS guards: one giant paste must not become one giant
	// ptmx write, and frames stay bounded.
	maxTerminalInputBytes = 256 * 1024
)

// Pooled binary frame buffers (payload + 1 tag byte), mirroring the WS
// handler so burst output never allocates per flush on either transport.
var wtTerminalFramePool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 64*1024+1)
		return &buf
	},
}

func (s *Server) serveTerminal(str *webtransport.Stream, w *streamWriter, id string) {
	if id != "" {
		// No re-attach in v1: a PTY tied to a dead session cannot migrate,
		// so clients always spawn fresh (Phase 6 recreates explicitly).
		_ = w.sendErrorf("Terminal streams always spawn fresh; stream id must be empty.")
		return
	}
	reqFrame, err := DecodeFrame(str)
	if err != nil {
		return // client gone before requesting
	}
	var req consolev1.WtTerminalRequest
	if err := wtProtoUnmarshal.Unmarshal(reqFrame.Payload, &req); err != nil {
		_ = w.sendErrorf("Invalid terminal spawn request.")
		return
	}
	params, err := services.ResolveTerminalSpawnParams(
		req.GetCwd(), req.GetShell(), req.GetLabel(),
		int(req.GetCols()), int(req.GetRows()), req.GetBinary(),
	)
	if err != nil {
		_ = w.sendErrorf("%s", err.Error())
		return
	}
	session, err := s.deps.PTY.Spawn(params)
	if err != nil {
		_ = w.sendErrorf("%s", err.Error())
		return
	}
	defer s.deps.PTY.Kill(session.ID)

	sendServerMessage := func(msg *consolev1.TerminalServerMessage) {
		raw, err := wtProtoMarshal.Marshal(msg)
		if err != nil {
			return
		}
		_ = w.send(&consolev1.WtFrame{Type: terminalMessageType(msg), Payload: raw})
	}
	sendError := func(text string) {
		sendServerMessage(&consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Error{
			Error: &consolev1.TerminalError{Message: text},
		}})
	}

	// JSON compat: stream-decode PTY bytes to text so multibyte sequences
	// split across coalesced frames survive. Binary clients skip decoding
	// entirely, same as the WS path.
	decoder := services.NewUTF8Decoder()
	session.SetCallbacks(func(chunk []byte) {
		if params.Binary {
			bufPtr := wtTerminalFramePool.Get().(*[]byte)
			buf := *bufPtr
			if cap(buf) < len(chunk)+1 {
				buf = make([]byte, len(chunk)+1)
			} else {
				buf = buf[:len(chunk)+1]
			}
			buf[0] = terminalOutputFrameTag
			copy(buf[1:], chunk)
			_ = w.send(&consolev1.WtFrame{Type: "output", Payload: buf})
			*bufPtr = buf[:0]
			wtTerminalFramePool.Put(bufPtr)
			return
		}
		if text := decoder.Decode(chunk); text != "" {
			sendServerMessage(&consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Output{
				Output: &consolev1.TerminalOutput{Data: text},
			}})
		}
	}, func(code int) {
		code32 := int32(code)
		sendServerMessage(&consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Exit{
			Exit: &consolev1.TerminalExit{Code: &code32},
		}})
	})
	sendServerMessage(&consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Spawned{
		Spawned: &consolev1.TerminalSpawned{
			Id: session.ID, Pid: int32(session.Pid()), Cwd: params.Cwd, Shell: params.Shell,
			Cols: int32(params.Cols), Rows: int32(params.Rows),
		},
	}})

	for {
		frame, err := DecodeFrame(str)
		if err != nil {
			return // client gone; deferred Kill reaps the PTY
		}
		switch frame.GetType() {
		case "input":
			if len(frame.GetPayload()) > maxTerminalInputBytes {
				sendError("Input too large.")
				continue
			}
			_ = session.Write(frame.GetPayload())
		case "resize":
			var resize consolev1.TerminalResize
			if err := wtProtoUnmarshal.Unmarshal(frame.GetPayload(), &resize); err != nil {
				sendError("Invalid resize frame.")
				continue
			}
			_ = session.Resize(int(resize.GetCols()), int(resize.GetRows()))
		case "kill":
			s.deps.PTY.Kill(session.ID)
			return
		default:
			sendError("Unknown terminal frame type: " + frame.GetType())
		}
	}
}

// terminalMessageType names a server message the way the WS wire does:
// the oneof field name, so frame types stay identical across transports.
func terminalMessageType(msg *consolev1.TerminalServerMessage) string {
	switch msg.GetEvent().(type) {
	case *consolev1.TerminalServerMessage_Spawned:
		return "spawned"
	case *consolev1.TerminalServerMessage_Output:
		return "output"
	case *consolev1.TerminalServerMessage_Exit:
		return "exit"
	case *consolev1.TerminalServerMessage_Error:
		return "error"
	default:
		return "error"
	}
}
