package hostc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

// webSocketStream is one public WebSocket bridged to a WebSocket on the
// local server.
type webSocketStream struct {
	client  *Client
	id      uint32
	open    OpenMessage
	started time.Time

	incoming *queue[Frame]

	mu      sync.Mutex
	socket  *websocket.Conn
	settled bool
	ended   bool
}

func newWebSocketStream(c *Client, id uint32, open OpenMessage) *webSocketStream {
	return &webSocketStream{
		client:   c,
		id:       id,
		open:     open,
		started:  time.Now(),
		incoming: newQueue[Frame](),
	}
}

// start opens the local WebSocket.
func (s *webSocketStream) start() {
	local := *s.client.Target
	scheme := "ws"
	if local.Scheme == "https" {
		scheme = "wss"
	}
	// The path is appended to the target host and never used to reach
	// another host: a path like //other-host/ must not leave the target.
	target := scheme + "://" + local.Host + s.open.Path
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host != local.Host {
		s.fail("request path escapes the target")
		return
	}

	header := http.Header{}
	for _, h := range stripWebSocketHandshake(toLocalRequestHeaders(s.open.Headers, s.client.publicOrigin, &local)) {
		header.Add(h[0], h[1])
	}
	dialer := *s.client.dialer()
	dialer.Subprotocols = s.open.WebSocket
	conn, resp, err := dialer.DialContext(context.Background(), target, header)
	if err != nil {
		if resp != nil {
			s.reject(resp)
			return
		}
		s.fail(fmt.Sprintf("local server: %v", err))
		return
	}

	s.mu.Lock()
	s.socket = conn
	s.mu.Unlock()

	s.settle(101, conn.Subprotocol())
	go s.readLocal(conn)
	go s.writeLocal(conn)
}

// readLocal forwards local messages to the tunnel server.
func (s *webSocketStream) readLocal(conn *websocket.Conn) {
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			s.localClosed(err)
			return
		}
		if msgType == websocket.TextMessage {
			s.client.send(FrameText, s.id, data)
		} else {
			s.client.send(FrameData, s.id, data)
		}
	}
}

// writeLocal forwards the tunnel server's messages to the local socket.
func (s *webSocketStream) writeLocal(conn *websocket.Conn) {
	for {
		frame, ok := s.incoming.pop()
		if !ok {
			return
		}
		switch frame.Type {
		case FrameData:
			if err := conn.WriteMessage(websocket.BinaryMessage, frame.Payload); err != nil {
				return
			}
		case FrameText:
			if err := conn.WriteMessage(websocket.TextMessage, frame.Payload); err != nil {
				return
			}
		case FrameEnd:
			closeMsg := decodeClose(frame.Payload)
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(int(closeMsg.Code), closeMsg.Reason))
			return
		case FrameReset:
			_ = conn.Close()
			return
		}
	}
}

// receive handles a frame addressed to this stream.
func (s *webSocketStream) receive(f Frame) {
	switch f.Type {
	case FrameData, FrameText, FrameEnd:
		s.incoming.push(f)
	case FrameReset:
		s.abort()
	default:
		s.client.closeWith(CloseProtocolError, fmt.Sprintf("unexpected frame type %d for a WebSocket stream", f.Type))
	}
}

// localClosed reports the local socket's end of stream to the tunnel server.
func (s *webSocketStream) localClosed(err error) {
	code := uint16(1006)
	reason := ""
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		code = uint16(closeErr.Code)
		reason = closeErr.Text
	}
	s.mu.Lock()
	already := s.ended
	s.ended = true
	socket := s.socket
	s.mu.Unlock()
	if !already {
		s.client.send(FrameEnd, s.id, encodeClose(CloseMessage{Code: code, Reason: reason}))
		s.client.removeStream(s.id)
		s.client.log(RequestLog{
			Method:    "GET",
			Path:      s.open.Path,
			Status:    101,
			Duration:  time.Since(s.started),
			WebSocket: true,
		})
	}
	if socket != nil {
		_ = socket.Close()
	}
}

// settle records the handshake outcome.
func (s *webSocketStream) settle(status int, protocol string) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	s.mu.Unlock()
	s.client.sendJSON(FrameHead, s.id, HeadMessage{
		Status:   status,
		Headers:  HeaderList{},
		Body:     false,
		Protocol: protocol,
	})
}

// reject reports that the local server refused the upgrade.
func (s *webSocketStream) reject(resp *http.Response) {
	headers := stripHopByHop(headerList(resp.Header), "content-length")
	s.client.sendJSON(FrameHead, s.id, HeadMessage{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    false,
	})
	resp.Body.Close()
	s.abort()
}

// fail reports a local failure to the tunnel server.
func (s *webSocketStream) fail(reason string) {
	s.client.send(FrameReset, s.id, []byte(reason))
	s.abort()
}

// abort tears the local socket down without telling the server.
func (s *webSocketStream) abort() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	socket := s.socket
	s.mu.Unlock()
	s.incoming.close()
	if socket != nil {
		_ = socket.Close()
	}
	s.client.removeStream(s.id)
}
