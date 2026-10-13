// Per-stream write helpers: serialized WtFrame emission plus the shared
// error-frame shape (protojson ErrorPayload, the same type the SSE error
// event carries).
package wt

import (
	"fmt"
	"sync"

	webtransport "github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

var wtProtoMarshal = protojson.MarshalOptions{}
var wtProtoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// streamWriter serializes WtFrame writes on one stream. QUIC streams allow
// concurrent Read+Write but concurrent Writes need external framing, so one
// mutex guards the encode+write pair.
type streamWriter struct {
	mu  sync.Mutex
	str *webtransport.Stream
}

func (w *streamWriter) send(frame *consolev1.WtFrame) error {
	raw, err := EncodeFrame(frame)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.str.Write(raw)
	return err
}

// sendEvent emits one event frame: the SSE-identical body bytes under the
// event name, sequenced for CHAT_RUN resume.
func (w *streamWriter) sendEvent(seq int64, eventType string, payload []byte) error {
	return w.send(&consolev1.WtFrame{Seq: uint64(seq), Type: eventType, Payload: payload})
}

// sendErrorf emits a terminal error frame and is the only error path once a
// stream is accepted: pre-accept failures stay HTTP statuses so clients can
// tell them apart from stream errors.
func (w *streamWriter) sendErrorf(format string, args ...any) error {
	raw, err := wtProtoMarshal.Marshal(&consolev1.ErrorPayload{Message: fmt.Sprintf(format, args...)})
	if err != nil {
		return err
	}
	return w.send(&consolev1.WtFrame{Type: "error", Payload: raw})
}
