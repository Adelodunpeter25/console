// DEVICE_STREAM streams: a viewer on a booted device, reusing
// services.PumpStream via a StreamConn adapter — the same template the WS
// route uses, so the device protocol is identical on both transports:
//
//	server -> viewer  text    DeviceStreamMeta  (frame type "text")
//	server -> viewer  binary  [tag] payload     (frame type "binary")
//	viewer -> server  text    sdk.Input JSON    (frame type "text")
//	plus a ping frame every ~54s, mirroring the WS ping cadence.
//
// Attach happens before streaming (OpenStream errors stay error frames,
// like the route's pre-upgrade HTTP errors).
package wt

import (
	"context"
	"time"

	webtransport "github.com/quic-go/webtransport-go"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func (s *Server) serveDeviceStream(ctx context.Context, str *webtransport.Stream, w *streamWriter, id string) {
	if id != "" {
		_ = w.sendErrorf("Device stream id must be empty.")
		return
	}
	reqFrame, err := DecodeFrame(str)
	if err != nil {
		return // client gone before requesting
	}
	var req consolev1.WtDeviceStreamRequest
	if err := wtProtoUnmarshal.Unmarshal(reqFrame.Payload, &req); err != nil {
		_ = w.sendErrorf("Invalid device stream request.")
		return
	}
	if req.GetDeviceId() == "" {
		_ = w.sendErrorf("Device stream request device_id is required.")
		return
	}
	// Bounded attach: without a simulator nearby this fails fast, and a
	// hung simulator handshake must not park a stream handler forever.
	attachCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := s.deps.Devices.OpenStream(attachCtx, req.GetPlatform(), req.GetDeviceId())
	if err != nil {
		_ = w.sendErrorf("Cannot open device stream: %s", err.Error())
		return
	}
	// PumpStream has no context hook, so wait for either its completion or
	// shutdown: returning closes the stream (deferred in serveStream),
	// which fails the pump's blocked reads/writes and reaps it. No leak
	// either way.
	done := make(chan struct{})
	go func() {
		defer close(done)
		services.PumpStream(&wtDeviceConn{str: str, w: w}, st)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// wtDeviceConn adapts a WT stream to services.StreamConn. Reads skip
// non-text frames the way the WS adapter skips non-text messages; writes
// go through the shared frame writer so output and ping frames serialize
// with each other.
type wtDeviceConn struct {
	str *webtransport.Stream
	w   *streamWriter
}

func (c *wtDeviceConn) WriteText(data []byte) error {
	return c.w.send(&consolev1.WtFrame{Type: "text", Payload: data})
}

func (c *wtDeviceConn) WriteBinary(data []byte) error {
	return c.w.send(&consolev1.WtFrame{Type: "binary", Payload: data})
}

func (c *wtDeviceConn) WritePing() error {
	return c.w.send(&consolev1.WtFrame{Type: "ping"})
}

func (c *wtDeviceConn) WriteClose() error {
	return c.str.Close()
}

func (c *wtDeviceConn) ReadText() ([]byte, error) {
	for {
		frame, err := DecodeFrame(c.str)
		if err != nil {
			return nil, err
		}
		if frame.GetType() == "text" {
			return frame.GetPayload(), nil
		}
	}
}
