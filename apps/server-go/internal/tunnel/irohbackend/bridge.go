//go:build iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package irohbackend

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"
)

// dialTimeout bounds the bridge's dial to the loopback target.
const dialTimeout = 5 * time.Second

// roundTrip replays an accepted request against the loopback target on
// its own TCP connection, the way the relay's session.go does for a
// visitor's request.
func roundTrip(req *http.Request, target string) (*http.Response, error) {
	d := net.Dialer{Timeout: dialTimeout}
	upstream, err := d.DialContext(req.Context(), "tcp", target)
	if err != nil {
		return nil, fmt.Errorf("dial target: %w", err)
	}
	defer upstream.Close()

	// http.Request.Write wants a well-formed request line; ReadRequest
	// leaves RequestURI set, which Write refuses to re-write — strip it.
	uri := req.RequestURI
	req.RequestURI = ""
	if req.Host == "" {
		req.Host = uri
	}
	if err := req.Write(upstream); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}
