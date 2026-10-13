// PORT_TUNNEL streams: raw TCP proxied as a reliable QUIC stream,
// transposing the WS tunnel byte for byte. No WtFrame envelope here — a
// tunnel is a byte pipe, and framing would corrupt TCP semantics (an HTTP
// upstream's response must arrive verbatim). The stream id must be empty;
// the first frame carries WtTunnelRequest.
package wt

import (
	"net"
	"strconv"
	"time"

	webtransport "github.com/quic-go/webtransport-go"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func (s *Server) serveTunnel(str *webtransport.Stream, w *streamWriter, id string) {
	if id != "" {
		_ = w.sendErrorf("Tunnel stream id must be empty.")
		return
	}
	reqFrame, err := DecodeFrame(str)
	if err != nil {
		return // client gone before requesting
	}
	var req consolev1.WtTunnelRequest
	if err := wtProtoUnmarshal.Unmarshal(reqFrame.Payload, &req); err != nil {
		_ = w.sendErrorf("Invalid tunnel request.")
		return
	}
	port := int(req.GetPort())
	if port < 1024 || port > 65535 {
		_ = w.sendErrorf("Invalid port.")
		return
	}
	target, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		_ = w.sendErrorf("Upstream connect failed.")
		return
	}
	defer target.Close()
	done := make(chan struct{}, 2)
	// TCP -> stream, raw.
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, rerr := target.Read(buf)
			if n > 0 {
				if _, werr := str.Write(buf[:n]); werr != nil {
					break
				}
			}
			if rerr != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	// Stream -> TCP, raw.
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, rerr := str.Read(buf)
			if n > 0 {
				if _, werr := target.Write(buf[:n]); werr != nil {
					break
				}
			}
			if rerr != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	<-done
	// Give the other direction a moment to drain in-flight bytes (e.g. an
	// HTTP/1.0 upstream that closes right after responding) before tearing
	// the socket down — same dance as the WS tunnel.
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}
