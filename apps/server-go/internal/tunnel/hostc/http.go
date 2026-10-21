package hostc

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// httpStream is one public HTTP request forwarded to the local server.
type httpStream struct {
	client  *Client
	id      uint32
	open    OpenMessage
	started time.Time

	credit *credit
	body   *queue[[]byte]

	mu       sync.Mutex
	finished bool
}

func newHTTPStream(c *Client, id uint32, open OpenMessage) *httpStream {
	return &httpStream{
		client:  c,
		id:      id,
		open:    open,
		started: time.Now(),
		credit:  newCredit(StreamWindowBytes),
		body:    newQueue[[]byte](),
	}
}

// start issues the request against the local target.
func (s *httpStream) start() {
	local := *s.client.Target
	local.Path = ""
	local.RawQuery = ""
	local.Fragment = ""
	target := local.Scheme + "://" + local.Host + s.open.Path

	var body io.Reader
	if s.open.Body {
		reader, writer := io.Pipe()
		go s.pumpBody(writer)
		body = reader
	}

	req, err := http.NewRequest(s.open.Method, target, body)
	if err != nil {
		s.fail(502, fmt.Sprintf("bad request: %v", err))
		return
	}
	for _, h := range toLocalRequestHeaders(s.open.Headers, s.client.publicOrigin, &local) {
		if strings.EqualFold(h[0], "host") {
			req.Host = h[1]
			continue
		}
		req.Header.Add(h[0], h[1])
	}
	go s.respond(req)
}

// pumpBody feeds the request body to the local server and returns the
// window to the tunnel server as the bytes are accepted.
func (s *httpStream) pumpBody(writer *io.PipeWriter) {
	defer writer.Close()
	for {
		payload, ok := s.body.pop()
		if !ok {
			return
		}
		if _, err := writer.Write(payload); err != nil {
			return
		}
		s.client.send(FrameWindow, s.id, EncodeWindow(uint32(len(payload))))
	}
}

// respond forwards the local response back through the tunnel.
func (s *httpStream) respond(req *http.Request) {
	resp, err := s.client.localClient().Do(req)
	if err != nil {
		s.fail(502, fmt.Sprintf("local server: %v", err))
		return
	}
	headers := toPublicResponseHeaders(
		stripHopByHop(headerList(resp.Header)),
		s.client.publicOrigin,
		s.client.Target,
	)
	hasBody := responseHasBody(s.open.Method, resp.StatusCode)
	s.client.sendJSON(FrameHead, s.id, HeadMessage{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    hasBody,
	})
	if !hasBody {
		resp.Body.Close()
		s.finish(resp.StatusCode)
		return
	}
	buf := make([]byte, MaxChunkBytes)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			part := buf[:n]
			// The tunnel server's window bounds how much may be in flight.
			if !s.credit.take(len(part)) {
				resp.Body.Close()
				return
			}
			s.client.send(FrameData, s.id, part)
		}
		if readErr != nil {
			if readErr != io.EOF {
				s.fail(resp.StatusCode, fmt.Sprintf("local response failed: %v", readErr))
				return
			}
			break
		}
	}
	s.client.send(FrameEnd, s.id, nil)
	s.finish(resp.StatusCode)
}

// receive handles a frame addressed to this stream.
func (s *httpStream) receive(f Frame) {
	switch f.Type {
	case FrameData:
		payload := make([]byte, len(f.Payload))
		copy(payload, f.Payload)
		s.body.push(payload)
	case FrameEnd:
		s.body.close()
	case FrameReset:
		s.abort()
	case FrameWindow:
		n, err := decodeWindow(f.Payload)
		if err != nil {
			s.client.closeWith(CloseProtocolError, err.Error())
			return
		}
		s.credit.grant(int(n))
	default:
		s.client.closeWith(CloseProtocolError, fmt.Sprintf("unexpected frame type %d for an HTTP stream", f.Type))
	}
}

// abort tears the local request down without telling the server.
func (s *httpStream) abort() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.mu.Unlock()
	s.credit.close()
	s.body.close()
	s.client.removeStream(s.id)
}

// fail reports a local failure to the tunnel server.
func (s *httpStream) fail(status int, reason string) {
	s.client.send(FrameReset, s.id, []byte(reason))
	s.finish(status)
}

// finish closes the stream and records the request.
func (s *httpStream) finish(status int) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.mu.Unlock()
	s.credit.close()
	s.body.close()
	s.client.removeStream(s.id)
	s.client.log(RequestLog{
		Method:   s.open.Method,
		Path:     s.open.Path,
		Status:   status,
		Duration: time.Since(s.started),
	})
}
