//go:build iroh && linux && (amd64 || arm64)

package irohbackend

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	iroh "git.coopcloud.tech/decentral1se/iroh-go"
)

// errClosed is returned by reads and writes once the stream is shut.
var errClosed = errors.New("iroh: stream closed")

// readChunk is the largest Read the bridge asks the FFI for at once.
const readChunk = 32 << 10

// streamConn adapts one bidirectional iroh stream to net.Conn.
//
// Reads map RecvStream.Read onto an io.Reader's contract. The binding's
// shapes (verified in the spike) are:
//
//	(data, nil)  -> peer sent data: return it
//	(data, err)  -> data still counts: drain it, then surface the error
//	(empty, nil) -> the peer called Finish and we drained everything: EOF
//	(empty, err) -> timeout or stream closed: surface iroh.As(err)
//
// That EOF case is the one net/http's request reader needs: the client
// half-closes after sending the request, which must read as end-of-body,
// not as an error and not as silence.
type streamConn struct {
	recv *iroh.RecvStream
	send *iroh.SendStream

	readMu    sync.Mutex
	writeMu   sync.Mutex
	deadline  time.Time
	closeOnce sync.Once
	closed    chan struct{}
}

func newStreamConn(recv *iroh.RecvStream, send *iroh.SendStream) *streamConn {
	return &streamConn{recv: recv, send: send, closed: make(chan struct{})}
}

// Read implements io.Reader over the receiving half of the stream.
func (c *streamConn) Read(buf []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, errClosed
	default:
	}
	if c.deadlineReached() {
		return 0, os.ErrDeadlineExceeded
	}
	size := len(buf)
	if size > readChunk {
		size = readChunk
	}
	data, err := c.recv.Read(uint32(size))
	n := copy(buf, data)
	switch {
	case n > 0:
		// Data wins over an error on the same call: report it next time.
		return n, nil
	case err == nil:
		return 0, io.EOF
	default:
		return 0, iroh.As(err)
	}
}

// Write implements io.Writer over the sending half of the stream.
func (c *streamConn) Write(buf []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, errClosed
	default:
	}
	if len(buf) == 0 {
		return 0, nil
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	// WriteAll retries short writes internally; buf larger than what the
	// FFI accepts in one call is split here.
	const writeChunk = 1 << 20
	written := 0
	for len(buf) > 0 {
		n := len(buf)
		if n > writeChunk {
			n = writeChunk
		}
		if err := c.send.WriteAll(buf[:n]); err != nil {
			return written, iroh.As(err)
		}
		written += n
		buf = buf[n:]
	}
	return written, nil
}

// Close finishes the sending side (so the peer reads the whole response)
// and stops the receiving side. Returning to a half-closed stream later is not supported; HTTP
// requests are one-shot per stream by design.
func (c *streamConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.send.Finish()
		_ = c.recv.Stop(0)
	})
	return nil
}

// HalfClose ends only the sending side: the peer's reads see EOF while our
// reads keep working. Unused today (requests half-close from the client),
// but it is the primitive a request-pipelining upgrade would need.
func (c *streamConn) HalfClose() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return iroh.As(c.send.Finish())
}

func (c *streamConn) deadlineReached() bool {
	return !c.deadline.IsZero() && time.Now().After(c.deadline)
}

func (c *streamConn) LocalAddr() net.Addr                { return address{} }
func (c *streamConn) RemoteAddr() net.Addr               { return address{} }
func (c *streamConn) SetDeadline(t time.Time) error      { c.deadline = t; return nil }
func (c *streamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *streamConn) SetWriteDeadline(t time.Time) error { return nil }

// address satisfies net.Addr for an iroh peer. A node ID is a public key,
// not an IP, so there is no meaningful address string; iroh's own logs
// carry the connection details.
type address struct{}

func (address) Network() string { return "iroh" }
func (address) String() string  { return "iroh" }

var _ net.Conn = (*streamConn)(nil)
