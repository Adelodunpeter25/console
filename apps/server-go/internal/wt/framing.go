// WebTransport stream framing: each bidi stream opens with a header,
// [uvarint kind][uvarint id_len][id], then carries repeated length-prefixed
// WtFrames (see frame.go). The header routes the stream before any payload
// is read, so malformed headers close the stream without an error frame.
package wt

import (
	"fmt"
	"io"

	"github.com/quic-go/quic-go/quicvarint"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

// maxStreamIDLen bounds the stream id (session/device ids, never bulk data).
const maxStreamIDLen = 512

// maxStreamKind is the highest WtStreamKind the server dispatches. Unknown
// kinds fail closed so a newer client can never drive an old server into a
// half-understood stream.
const maxStreamKind = uint64(consolev1.WtStreamKind_WT_STREAM_KIND_DEVICE_STREAM)

// WriteStreamHeader writes one stream header.
func WriteStreamHeader(w io.Writer, kind consolev1.WtStreamKind, id string) error {
	if uint64(kind.Number()) > maxStreamKind || kind == consolev1.WtStreamKind_WT_STREAM_KIND_UNSPECIFIED {
		return fmt.Errorf("invalid WT stream kind %d.", kind.Number())
	}
	if len(id) > maxStreamIDLen {
		return fmt.Errorf("WT stream id of %d bytes exceeds the %d-byte cap.", len(id), maxStreamIDLen)
	}
	var header []byte
	header = quicvarint.Append(header, uint64(kind.Number()))
	header = quicvarint.Append(header, uint64(len(id)))
	header = append(header, id...)
	_, err := w.Write(header)
	return err
}

// ReadStreamHeader reads one stream header. Varints are read byte by byte
// with no buffering: a bufio.Reader here would over-consume the frame that
// already arrived behind the header and strand it in a discarded buffer.
func ReadStreamHeader(r io.Reader) (consolev1.WtStreamKind, string, error) {
	br := quicvarint.NewReader(r)
	kindNum, err := quicvarint.Read(br)
	if err != nil {
		return 0, "", err
	}
	if kindNum == 0 || kindNum > maxStreamKind {
		return 0, "", fmt.Errorf("invalid WT stream kind %d.", kindNum)
	}
	idLen, err := quicvarint.Read(br)
	if err != nil {
		return 0, "", err
	}
	if idLen > maxStreamIDLen {
		return 0, "", fmt.Errorf("WT stream id length %d exceeds the %d-byte cap.", idLen, maxStreamIDLen)
	}
	id := make([]byte, idLen)
	if _, err := io.ReadFull(br, id); err != nil {
		return 0, "", err
	}
	return consolev1.WtStreamKind(kindNum), string(id), nil
}
