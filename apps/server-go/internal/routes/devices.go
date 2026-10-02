// Device simulator routes. Logic lives in services.DeviceService; this file
// only parses requests, maps errors to status codes, and adapts fasthttp's
// websocket to services.StreamConn.
package routes

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	deviceWriteWait = 10 * time.Second
	deviceMaxInput  = 1 << 20
)

func deviceStatus(err error) int {
	switch {
	case errors.Is(err, services.ErrDeviceInvalidPlatform), errors.Is(err, services.ErrDeviceBadRequest):
		return fiber.StatusBadRequest
	case errors.Is(err, services.ErrDeviceNotFound):
		return fiber.StatusNotFound
	case errors.Is(err, services.ErrDeviceUnsupported):
		return fiber.StatusNotImplemented
	}
	return fiber.StatusBadGateway
}

func deviceFail(c *fiber.Ctx, err error) error {
	return c.Status(deviceStatus(err)).JSON(fiber.Map{"success": false, "error": err.Error()})
}

func deviceOK(c *fiber.Ctx, data any) error {
	return c.JSON(fiber.Map{"success": true, "data": data})
}

func registerDeviceRoutes(app *fiber.App, svc *services.DeviceService) {
	h := app.Group("/api/devices")

	h.Get("/", func(c *fiber.Ctx) error {
		devs, err := svc.List(c.Context())
		if err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, devs)
	})

	h.Get("/diagnostics", func(c *fiber.Ctx) error {
		return deviceOK(c, svc.Diagnostics(c.Context()))
	})

	h.Post("/shutdown-all", func(c *fiber.Ctx) error {
		if err := svc.ShutdownAll(c.Context()); err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, fiber.Map{})
	})

	h.Post("/:id/boot", func(c *fiber.Ctx) error {
		if err := svc.Boot(c.Query("platform"), c.Params("id")); err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, fiber.Map{})
	})

	h.Post("/:id/shutdown", func(c *fiber.Ctx) error {
		if err := svc.Shutdown(c.Context(), c.Query("platform"), c.Params("id")); err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, fiber.Map{})
	})

	h.Post("/:id/open-app", func(c *fiber.Ctx) error {
		var body types.DeviceOpenAppRequest
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return deviceFail(c, errors.Join(services.ErrDeviceBadRequest, err))
		}
		out, err := svc.OpenApp(c.Context(), c.Query("platform"), c.Params("id"), body.App)
		if err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, fiber.Map{"output": out})
	})

	h.Post("/:id/interact", func(c *fiber.Ctx) error {
		var action types.DeviceAction
		if err := json.Unmarshal(c.Body(), &action); err != nil {
			return deviceFail(c, errors.Join(services.ErrDeviceBadRequest, err))
		}
		if err := svc.Interact(c.Context(), c.Query("platform"), c.Params("id"), action); err != nil {
			return deviceFail(c, err)
		}
		return deviceOK(c, fiber.Map{})
	})

	h.Get("/:id/screenshot", func(c *fiber.Ctx) error {
		png, err := svc.Screenshot(c.Context(), c.Query("platform"), c.Params("id"))
		if err != nil {
			return deviceFail(c, err)
		}
		c.Set(fiber.HeaderContentType, "image/png")
		return c.Send(png)
	})

	h.Get("/:id/stream", func(c *fiber.Ctx) error {
		// Attach before upgrading so not-booted / bad platform stay HTTP errors.
		st, err := svc.OpenStream(c.Context(), c.Query("platform"), c.Params("id"))
		if err != nil {
			return deviceFail(c, err)
		}
		wsErr := upgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
			defer conn.Close()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("device stream panic", "panic", r)
				}
			}()
			services.PumpStream(newDeviceWSConn(conn), st)
		})
		if wsErr != nil {
			_ = st.Close()
			return deviceFail(c, errors.Join(services.ErrDeviceBadRequest, wsErr))
		}
		return nil
	})
}

// deviceWSConn adapts fasthttp/websocket to services.StreamConn.
type deviceWSConn struct {
	conn *websocket.Conn
}

func newDeviceWSConn(conn *websocket.Conn) *deviceWSConn {
	conn.SetReadLimit(deviceMaxInput)
	_ = conn.SetReadDeadline(time.Now().Add(services.DeviceStreamPongTTL))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(services.DeviceStreamPongTTL))
	})
	return &deviceWSConn{conn: conn}
}

func (w *deviceWSConn) write(kind int, data []byte) error {
	_ = w.conn.SetWriteDeadline(time.Now().Add(deviceWriteWait))
	return w.conn.WriteMessage(kind, data)
}

func (w *deviceWSConn) WriteText(data []byte) error   { return w.write(websocket.TextMessage, data) }
func (w *deviceWSConn) WriteBinary(data []byte) error { return w.write(websocket.BinaryMessage, data) }
func (w *deviceWSConn) WritePing() error              { return w.write(websocket.PingMessage, nil) }

func (w *deviceWSConn) WriteClose() error {
	return w.write(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func (w *deviceWSConn) ReadText() ([]byte, error) {
	for {
		typ, data, err := w.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		_ = w.conn.SetReadDeadline(time.Now().Add(services.DeviceStreamPongTTL))
		if typ == websocket.TextMessage {
			return data, nil
		}
	}
}
