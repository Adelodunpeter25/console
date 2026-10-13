// CHAT_RUN streams: start a run or attach to one, then pump hub frames as
// WtFrames until the run settles. Payloads are the exact SSE body bytes
// (run.WireFrame + encoding/json), so client parsers are shared across
// transports and only framing changes. No app-level pings: QUIC keepalive
// (15s, see the server QUIC config) owns dead-peer detection.
//
// Error mapping mirrors the REST routes: unknown session, double start,
// and attach-without-run all answer error frames (there are no HTTP
// statuses once the stream is accepted).
package wt

import (
	"encoding/json"
	"strings"

	webtransport "github.com/quic-go/webtransport-go"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
)

func (s *Server) serveChatRun(str *webtransport.Stream, w *streamWriter, id string) {
	if id == "" {
		_ = w.sendErrorf("Chat run stream id must be the session id.")
		return
	}
	reqFrame, err := DecodeFrame(str)
	if err != nil {
		return // client gone before requesting
	}
	var req consolev1.WtChatRunRequest
	if err := wtProtoUnmarshal.Unmarshal(reqFrame.Payload, &req); err != nil {
		_ = w.sendErrorf("Invalid chat run request.")
		return
	}
	if req.SessionId != "" && req.SessionId != id {
		_ = w.sendErrorf("Chat run request targets a different session.")
		return
	}
	switch r := req.GetRequest().(type) {
	case *consolev1.WtChatRunRequest_Start:
		s.startChatRun(w, id, r.Start)
	case *consolev1.WtChatRunRequest_Attach:
		s.attachChatRun(w, id, r.Attach)
	default:
		_ = w.sendErrorf("Chat run request is required.")
	}
}

func (s *Server) startChatRun(w *streamWriter, sessionID string, start *consolev1.WtChatRunStart) {
	if start == nil || strings.TrimSpace(start.GetText()) == "" {
		_ = w.sendErrorf("Field 'prompt' is required.")
		return
	}
	prompt := run.Prompt{
		Text: strings.TrimSpace(start.GetText()), ContextFiles: start.GetContextFiles(),
		ModelID: start.GetModelId(), Provider: start.GetProvider(),
		ApprovalMode: start.GetApprovalMode(), Thinking: start.GetThinkingLevel(),
	}
	for _, a := range start.GetAttachments() {
		prompt.Attachments = append(prompt.Attachments, run.Attachment{Data: a.GetData(), MimeType: a.GetMimeType()})
	}
	for _, a := range start.GetAnnotations() {
		prompt.Annotations = append(prompt.Annotations, loop.AnnotationFromProto(a))
	}
	hub, err := s.deps.Runs.StartRun(sessionID, prompt)
	if err != nil {
		// Same messages as the REST route: clients already display them.
		_ = w.sendErrorf("%s", err.Error())
		return
	}
	s.pumpChatRun(w, hub, nil)
}

func (s *Server) attachChatRun(w *streamWriter, sessionID string, attach *consolev1.WtChatRunAttach) {
	var since *int64
	if attach != nil && attach.Since != nil {
		since = attach.Since
	}
	if !s.deps.Runs.IsActive(sessionID) {
		_ = w.sendErrorf("No active run for session '%s'.", sessionID)
		return
	}
	hub := s.deps.Runs.Hub(sessionID)
	if hub == nil {
		_ = w.sendErrorf("No active run for session '%s'.", sessionID)
		return
	}
	s.pumpChatRun(w, hub, since)
}

// pumpChatRun mirrors the SSE pumpHub: replay, then live frames until the
// hub settles, draining bursts without a flush per frame. A write error
// means the client is gone; the server-side run continues.
func (s *Server) pumpChatRun(w *streamWriter, hub *run.Hub, since *int64) {
	id, ch, replay := hub.Subscribe(since)
	if ch == nil {
		_ = w.sendErrorf("Run already settled.")
		return
	}
	defer hub.Unsubscribe(id)
	for _, f := range replay {
		if err := sendChatFrame(w, f); err != nil {
			return
		}
	}
	for {
		f, ok := <-ch
		if !ok {
			return
		}
		if err := sendChatFrame(w, f); err != nil {
			return
		}
		for more := true; more; {
			select {
			case next, ok := <-ch:
				if !ok {
					return
				}
				if err := sendChatFrame(w, next); err != nil {
					return
				}
			default:
				more = false
			}
		}
	}
}

func sendChatFrame(w *streamWriter, f run.Frame) error {
	name, body, drop := run.WireFrame(f.Event)
	if drop {
		return nil
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return w.sendEvent(f.Seq, name, payload)
}
