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
)

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
	defer conn.Close()

	sendJSON := func(msg any) {
		data, _ := json.Marshal(msg)
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	session, err := ptyManager.Spawn(params)
	if err != nil {
		sendJSON(types.TerminalErrorMessage{Type: "error", Message: err.Error()})
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4000, "Spawn failed"))
		return
	}
	defer ptyManager.Kill(session.ID)

	session.OnData = func(chunk []byte) {
		if params.Binary {
			frame := make([]byte, len(chunk)+1)
			frame[0] = terminalOutputFrameTag
			copy(frame[1:], chunk)
			_ = conn.WriteMessage(websocket.BinaryMessage, frame)
		} else {
			sendJSON(fiber.Map{"type": "output", "data": sanitizeUTF8(chunk)})
		}
	}
	session.OnExit = func(code int) {
		sendJSON(types.TerminalExitMessage{Type: "exit", Code: code})
	}
	sendJSON(types.TerminalSpawnedMessage{
		Type: "spawned", ID: session.ID, Cwd: params.Cwd, Shell: params.Shell,
		Label: params.Label, Cols: params.Cols, Rows: params.Rows,
	})

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			slog.Debug("terminal socket closed", "session", session.ID, "error", err)
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
				_ = session.Write(data[1:])
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
