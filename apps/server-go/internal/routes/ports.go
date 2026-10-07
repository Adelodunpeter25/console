// Port routes (/api/ports/*) plus the tunnel WebSocket: raw TCP over
// WebSocket binary frames in both directions.
//
// Fifth domain on the shared protobuf schema. List, stream
// frames, forward responses, and unforward responses are built from
// console.v1 generated types with byte-identical output. The forward
// *request* keeps its lenient number-or-string parsing (see
// proto/console/v1/ports.proto), and the tunnel endpoint is untouched.
package routes

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
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

// clientPortToProto converts a registry row to the canonical wire type.
func clientPortToProto(p types.ClientPort) *consolev1.ForwardedPort {
	out := &consolev1.ForwardedPort{Port: int32(p.Port), Url: p.URL}
	if p.ProjectID != nil {
		out.ProjectId = p.ProjectID
	}
	return out
}

func portsToProto(list []types.ClientPort) []*consolev1.ForwardedPort {
	out := make([]*consolev1.ForwardedPort, 0, len(list))
	for _, p := range list {
		out = append(out, clientPortToProto(p))
	}
	return out
}

func sendPortList(c *fiber.Ctx, list []types.ClientPort) error {
	data, err := marshalProtoList(portsToProto(list))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": data})
}

func RegisterPortRoutes(app *fiber.App, registry *services.PortRegistry) {
	h := app.Group("/api/ports")

	h.Get("/stream", func(c *fiber.Ctx) error {
		host := hostFromHeader(c.Get("Host"))
		projectID := c.Query("projectId")
		return streamSSE(c, func(sse *sseStream) {
			events := registry.Subscribe()
			defer registry.Unsubscribe(events)
			// mustJSON semantics: a marshal failure sends "{}" rather than
			// dropping the snapshot (proto messages cannot fail to encode
			// in practice, but the stream must never emit a blank frame).
			snapshotOf := func() string {
				snapshot, err := marshalProtoList(portsToProto(registry.Snapshot(host, projectID)))
				if err != nil {
					return "{}"
				}
				return string(snapshot)
			}
			_ = sse.Send("ports", snapshotOf())
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-events:
					if err := sse.Send("ports", snapshotOf()); err != nil {
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
		return sendPortList(c, registry.List(host, c.Query("projectId")))
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
		raw, err := protoMarshal.Marshal(clientPortToProto(entry))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Delete("/:port", func(c *fiber.Ctx) error {
		port, err := strconv.Atoi(c.Params("port"))
		if err != nil {
			return fail400(c, fmt.Errorf("Port must be an integer."))
		}
		if !registry.Remove(port, c.Query("projectId")) {
			return fail400(c, fmt.Errorf("Port %d is not forwarded.", port))
		}
		raw, err := protoMarshal.Marshal(&consolev1.UnforwardPortResponse{Port: int32(port)})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
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
