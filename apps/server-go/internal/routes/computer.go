// WebSocket streaming endpoint for the Computer tab (/api/computer/ws)
// and embedded player page (/api/computer/player).
package routes

import (
	"context"
	_ "embed"
	"github.com/MoeForge/moego/json"
	"log/slog"
	"sync"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/computer"
)

//go:embed computer_player.html
var computerPlayerHTML string

var computerUpgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024 * 64,
	WriteBufferSize: 1024 * 256,
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		return true
	},
}

type computerInMsg struct {
	Type   string `json:"type"`
	X      int32  `json:"x,omitempty"`
	Y      int32  `json:"y,omitempty"`
	Mask   uint32 `json:"mask,omitempty"`
	Wheel  int32  `json:"wheel,omitempty"`
	Keysym uint32 `json:"keysym,omitempty"`
	Down   bool   `json:"down,omitempty"`
	Width  uint32 `json:"width,omitempty"`
	Height uint32 `json:"height,omitempty"`
}

// RegisterComputerRoutes registers the desktop streaming endpoints.
func RegisterComputerRoutes(app *fiber.App, svc *computer.Service) {
	if svc == nil {
		svc = computer.Default()
	}

	app.Get("/api/computer/player", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(computerPlayerHTML)
	})

	app.Get("/api/computer/ws", func(c *fiber.Ctx) error {
		if !websocket.FastHTTPIsWebSocketUpgrade(c.Context()) {
			return fiber.ErrUpgradeRequired
		}

		return computerUpgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
			defer conn.Close()

			var writeMu sync.Mutex
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			unsub, err := svc.Subscribe(ctx, func(kind uint32, data []byte) {
				writeMu.Lock()
				defer writeMu.Unlock()
				_ = conn.WriteMessage(websocket.BinaryMessage, data)
			})
			if err != nil {
				slog.Error("computer: subscribe failed", "err", err)
				return
			}
			defer unsub()

			for {
				msgType, msgBytes, err := conn.ReadMessage()
				if err != nil {
					break
				}

				if msgType == websocket.TextMessage {
					var in computerInMsg
					if err := json.Unmarshal(msgBytes, &in); err != nil {
						continue
					}

					switch in.Type {
					case "mouse":
						_ = svc.SendMouse(in.X, in.Y, in.Mask, in.Wheel)
					case "key":
						_ = svc.SendKey(in.Keysym, in.Down)
					case "resize":
						if in.Width > 0 && in.Height > 0 {
							_ = svc.Resize(in.Width, in.Height)
						}
					}
				}
			}
		})
	})
}
