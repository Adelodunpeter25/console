// Terminal WebSocket endpoint (/api/terminals). Port of
// apps/server/api/src/terminal/socket.route.ts: JSON frames
// {spawned|output|exit|error} server→client, {input|resize|kill}
// client→server, plus the ?proto=binary tag-byte framing.
package routes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	terminalOutputFrameTag = 0x01
	terminalInputFrameTag  = 0x01
	maxTerminalFrameBytes  = 1024 * 1024
	// Mirror Bun's write() guard: one giant paste must not become one giant
	// ptmx write.
	maxTerminalInputBytes = 256 * 1024
)

// Pooled binary frame buffers (payload + 1 tag byte). Coalesced flushes cap
// payloads at 64KB, so pooled buffers stay bounded and per-read allocations
// disappear under burst output.
var terminalFramePool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 64*1024+1)
		return &buf
	},
}

var upgrader = websocket.FastHTTPUpgrader{
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool { return true },
}

func registerTerminalRoutes(app *fiber.App, ptyManager *services.PtyManager) {
	app.Get("/api/terminals", func(c *fiber.Ctx) error {
		params, perr := parseSpawnParams(
			c.Query("cwd"), c.Query("shell"), c.Query("label"),
			c.Query("cols"), c.Query("rows"), c.Query("proto"))
		if perr != nil {
			return fiber.NewError(fiber.StatusBadRequest, perr.Error())
		}
		// Upgrade hijacks the connection; the handler owns it afterwards.
		err := upgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
			handleTerminalConn(conn, ptyManager, params)
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return nil
	})
}

func handleTerminalConn(conn *websocket.Conn, ptyManager *services.PtyManager, params types.TerminalSpawnParams) {
	var connMu sync.Mutex
	connClosed := false

	// closeConn is the only closer: flag + close under one lock so a
	// concurrent PTY-callback write can never race the close (fasthttp
	// panics, rather than errors, on use-after-close). Writes are
	// serialized on the same mutex: the PTY pump and the exit path both
	// write while the read loop may be closing.
	closeConn := func() {
		connMu.Lock()
		if !connClosed {
			connClosed = true
			_ = conn.Close()
		}
		connMu.Unlock()
	}
	defer closeConn()
	// One panicking socket must never take down the daemon.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("terminal socket panic", "panic", r)
		}
	}()

	sendJSON := func(msg any) {
		connMu.Lock()
		defer connMu.Unlock()
		if connClosed {
			return
		}
		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Debug("terminal send error", "error", err)
			connClosed = true
		}
	}
	sendBinary := func(chunk []byte) {
		bufPtr := terminalFramePool.Get().(*[]byte)
		buf := *bufPtr
		if cap(buf) < len(chunk)+1 {
			buf = make([]byte, len(chunk)+1)
		} else {
			buf = buf[:len(chunk)+1]
		}
		buf[0] = terminalOutputFrameTag
		copy(buf[1:], chunk)
		connMu.Lock()
		if !connClosed {
			if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
				slog.Debug("terminal binary send error", "error", err)
				connClosed = true
			}
		}
		connMu.Unlock()
		*bufPtr = buf[:0]
		terminalFramePool.Put(bufPtr)
	}

	session, err := ptyManager.Spawn(params)
	if err != nil {
		sendJSON(types.TerminalErrorMessage{Type: "error", Message: err.Error()})
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4000, "Spawn failed"))
		return
	}
	defer ptyManager.Kill(session.ID)

	// JSON compat: stream-decode PTY bytes to text so multibyte sequences
	// split across coalesced frames survive (mirrors Bun's TextDecoder
	// stream mode). Binary clients skip decoding entirely.
	// Backpressure: WS writes are synchronous, so a slow client throttles
	// flushes directly; the session queue itself is bounded at 8MB with
	// oldest-drop, and Pause/Resume remain available for explicit control.
	decoder := services.NewUTF8Decoder()

	session.SetCallbacks(func(chunk []byte) {
		if params.Binary {
			sendBinary(chunk)
		} else {
			text := decoder.Decode(chunk)
			if text != "" {
				sendJSON(fiber.Map{"type": "output", "data": text})
			}
		}
	}, func(code int) {
		sendJSON(types.TerminalExitMessage{Type: "exit", Code: code})
	})
	sendJSON(types.TerminalSpawnedMessage{
		Type: "spawned", ID: session.ID, Cwd: params.Cwd, Shell: params.Shell,
		Label: params.Label, Cols: params.Cols, Rows: params.Rows,
	})

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			slog.Debug("terminal socket closed", "session", session.ID, "error", err)
			connMu.Lock()
			connClosed = true
			connMu.Unlock()
			return
		}
		if len(data) > maxTerminalFrameBytes {
			sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Frame too large."})
			continue
		}
		switch msgType {
		case websocket.BinaryMessage:
			if !params.Binary {
				sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Binary frames require ?proto=binary."})
				continue
			}
			if len(data) == 0 {
				continue
			}
			if data[0] == terminalInputFrameTag {
				payload := data[1:]
				if len(payload) > maxTerminalInputBytes {
					sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Input too large."})
					continue
				}
				_ = session.Write(payload)
			} else {
				sendJSON(types.TerminalErrorMessage{Type: "error", Message: fmt.Sprintf("Unknown binary frame tag: %d", data[0])})
			}
		case websocket.TextMessage:
			var frame types.TerminalClientMessage
			if err := json.Unmarshal(data, &frame); err != nil {
				sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Invalid terminal frame: expected JSON."})
				continue
			}
			switch frame.Type {
			case "input":
				if len(frame.Data) > maxTerminalInputBytes {
					sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Input too large."})
					continue
				}
				_ = session.Write([]byte(frame.Data))
			case "resize":
				_ = session.Resize(frame.Cols, frame.Rows)
			case "kill":
				ptyManager.Kill(session.ID)
				return
			default:
				sendJSON(types.TerminalErrorMessage{Type: "error", Message: "Unknown terminal frame type: " + frame.Type})
			}
		}
	}
}

func parseSpawnParams(cwd, shell, label, cols, rows, proto string) (types.TerminalSpawnParams, error) {
	if len(cwd) > 4096 {
		return types.TerminalSpawnParams{}, fmt.Errorf("cwd too long")
	}
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			return types.TerminalSpawnParams{}, err
		}
	}
	if len(shell) > 1024 {
		return types.TerminalSpawnParams{}, fmt.Errorf("shell path too long")
	}
	if len(label) > 256 {
		return types.TerminalSpawnParams{}, fmt.Errorf("label too long")
	}
	if proto != "" && proto != "binary" {
		return types.TerminalSpawnParams{}, fmt.Errorf("Unsupported proto: %s", proto)
	}
	return types.TerminalSpawnParams{
		Cwd:    cwd,
		Shell:  shell,
		Cols:   clampInt(parseIntDefault(cols, 80), 1, 500),
		Rows:   clampInt(parseIntDefault(rows, 24), 1, 200),
		Label:  label,
		Binary: proto == "binary",
	}, nil
}

func parseIntDefault(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
