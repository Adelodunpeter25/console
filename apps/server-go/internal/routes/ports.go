// Port routes (/api/ports/*). Port of apps/server/api/src/routes/ports.ts
// plus the tunnel WebSocket (port-tunnel.socket.ts): raw TCP over WebSocket
// binary frames in both directions.
package routes

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func hostFromHeader(header string) string {
	if idx := strings.LastIndex(header, ":"); idx > 0 && !strings.HasSuffix(header, "]") {
		if _, err := strconv.Atoi(header[idx+1:]); err == nil {
			return header[:idx]
		}
	}
	if header == "" {
		return "localhost"
	}
	return header
}

func registerPortRoutes(app *fiber.App, registry *services.PortRegistry) {
	h := app.Group("/api/ports")

	h.Get("/stream", func(c *fiber.Ctx) error {
		host := hostFromHeader(c.Get("Host"))
		projectID := c.Query("projectId")
		return streamSSE(c, func(sse *sseStream) {
			events := registry.Subscribe()
			defer registry.Unsubscribe(events)
			_ = sse.Send("ports", mustJSON(registry.Snapshot(host, projectID)))
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-events:
					if err := sse.Send("ports", mustJSON(registry.Snapshot(host, projectID))); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})

	h.Get("/", func(c *fiber.Ctx) error {
		host := hostFromHeader(c.Get("Host"))
		return c.JSON(fiber.Map{"success": true, "data": registry.List(host, c.Query("projectId"))})
	})

	h.Post("/forward", func(c *fiber.Ctx) error {
		var body struct {
			Port      any     `json:"port"`
			ProjectID *string `json:"projectId"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fail400(c, fmt.Errorf("Request body must be valid JSON."))
		}
		var port int
		switch v := body.Port.(type) {
		case float64:
			port = int(v)
		case string:
			port, _ = strconv.Atoi(v)
		}
		projectID := ""
		if body.ProjectID != nil {
			projectID = *body.ProjectID
		}
		entry, err := registry.Forward(port, projectID)
		if err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": entry})
	})

	h.Delete("/:port", func(c *fiber.Ctx) error {
		port, err := strconv.Atoi(c.Params("port"))
		if err != nil {
			return fail400(c, fmt.Errorf("Port must be an integer."))
		}
		if !registry.Remove(port, c.Query("projectId")) {
			return fail400(c, fmt.Errorf("Port %d is not forwarded.", port))
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"port": port}})
	})

	// GET /api/ports/:port/tunnel — raw TCP over WebSocket binary frames.
	h.Get("/:port/tunnel", func(c *fiber.Ctx) error {
		port, perr := strconv.Atoi(c.Params("port"))
		if perr != nil || port < 1024 || port > 65535 {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid port")
		}
		wsErr := upgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
			defer conn.Close()
			target, derr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
			if derr != nil {
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4400, "Upstream connect failed"))
				return
			}
			defer target.Close()
			done := make(chan struct{}, 2)
			// TCP -> WS binary frames.
			go func() {
				buf := make([]byte, 64*1024)
				for {
					n, rerr := target.Read(buf)
					if n > 0 {
						if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
							break
						}
					}
					if rerr != nil {
						break
					}
				}
				done <- struct{}{}
			}()
			// WS -> TCP.
			go func() {
				for {
					msgType, data, rerr := conn.ReadMessage()
					if rerr != nil {
						break
					}
					if msgType == websocket.BinaryMessage {
						if _, werr := target.Write(data); werr != nil {
							break
						}
					}
				}
				done <- struct{}{}
			}()
			<-done
			// Give the other direction a moment to drain in-flight frames
			// (e.g. an HTTP/1.0 upstream that closes right after responding)
			// before tearing the socket down.
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
			}
		})
		if wsErr != nil {
			return fiber.NewError(fiber.StatusBadRequest, wsErr.Error())
		}
		return nil
	})
}
