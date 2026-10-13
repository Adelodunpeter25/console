// WATCH streams: fs/git/ports/notifications/script-run watches multiplexed
// by WtWatchTarget, one target per stream. Each mirrors its SSE route's
// snapshot and pump semantics; only framing changes (no app-level pings —
// QUIC keepalive owns dead-peer detection).
package wt

import (
	"context"
	"encoding/json"
	"time"

	webtransport "github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// gitStatusDebounce mirrors the SSE route: leading-edge throttle so
// continuous build/push events fold into one refresh instead of spawning
// git constantly (or never firing on a resetting trailing debounce).
const gitStatusDebounce = 1500 * time.Millisecond

// scriptRunPollInterval mirrors the SSE route's end-of-run detection.
const scriptRunPollInterval = 100 * time.Millisecond

func (s *Server) serveWatch(ctx context.Context, str *webtransport.Stream, w *streamWriter, id string) {
	if id != "" {
		// Watches are always fresh subscriptions; the request carries
		// every param, so a non-empty id is a client bug.
		_ = w.sendErrorf("Watch stream id must be empty.")
		return
	}
	reqFrame, err := DecodeFrame(str)
	if err != nil {
		return // client gone before requesting
	}
	var req consolev1.WtWatchRequest
	if err := wtProtoUnmarshal.Unmarshal(reqFrame.Payload, &req); err != nil {
		_ = w.sendErrorf("Invalid watch request.")
		return
	}
	switch req.GetTarget() {
	case consolev1.WtWatchTarget_WT_WATCH_TARGET_FS:
		if req.GetPath() == "" {
			_ = w.sendErrorf("Watch request path is required.")
			return
		}
		s.pumpFsWatch(ctx, w, req.GetPath())
	case consolev1.WtWatchTarget_WT_WATCH_TARGET_GIT_STATUS:
		if req.GetPath() == "" {
			_ = w.sendErrorf("Watch request path is required.")
			return
		}
		s.pumpGitWatch(ctx, w, req.GetPath())
	case consolev1.WtWatchTarget_WT_WATCH_TARGET_PORTS:
		s.pumpPortsWatch(ctx, w, req.GetHost(), req.GetProjectId())
	case consolev1.WtWatchTarget_WT_WATCH_TARGET_NOTIFICATIONS:
		s.pumpNotificationsWatch(ctx, w)
	case consolev1.WtWatchTarget_WT_WATCH_TARGET_SCRIPT_RUN:
		if req.GetProjectId() == "" || req.GetRunId() == "" {
			_ = w.sendErrorf("Watch request project_id and run_id are required.")
			return
		}
		s.pumpScriptRunWatch(ctx, w, req.GetProjectId(), req.GetRunId())
	default:
		_ = w.sendErrorf("Unknown watch target.")
	}
}

// sendProtoEvent marshals one generated message as an event frame.
func (w *streamWriter) sendProtoEvent(eventType string, msg proto.Message) error {
	raw, err := wtProtoMarshal.Marshal(msg)
	if err != nil {
		return err
	}
	return w.sendEvent(0, eventType, raw)
}

func marshalProtoJSONList[T proto.Message](items []T) ([]byte, error) {
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := wtProtoMarshal.Marshal(item)
		if err != nil {
			return nil, err
		}
		elements = append(elements, raw)
	}
	return json.Marshal(elements)
}

func (s *Server) pumpFsWatch(ctx context.Context, w *streamWriter, path string) {
	s.deps.Watch.Watch(path)
	events := s.deps.Watch.Subscribe(path)
	defer s.deps.Watch.Unsubscribe(events)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			msg := &consolev1.FsChangeEvent{Type: evt.Type, ProjectPath: evt.ProjectPath}
			if evt.EventPath != "" {
				msg.EventPath = &evt.EventPath
			}
			if err := w.sendProtoEvent("fsChange", msg); err != nil {
				return
			}
		}
	}
}

func (s *Server) pumpGitWatch(ctx context.Context, w *streamWriter, repoPath string) {
	s.deps.Watch.Watch(repoPath)
	events := s.deps.Watch.Subscribe(repoPath)
	defer s.deps.Watch.Unsubscribe(events)

	sendStatus := func() error {
		return w.sendProtoEvent("gitStatus", services.GitSummaryToProto(s.deps.Git.GetGitStatus(repoPath)))
	}
	if err := sendStatus(); err != nil {
		return
	}
	debounce := time.NewTimer(gitStatusDebounce)
	debounce.Stop()
	pending := false
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-events:
			if !ok {
				return
			}
			if !pending {
				pending = true
				debounce.Reset(gitStatusDebounce)
			}
		case <-debounce.C:
			pending = false
			if err := sendStatus(); err != nil {
				return
			}
		}
	}
}

func (s *Server) pumpPortsWatch(ctx context.Context, w *streamWriter, host, projectID string) {
	if host == "" {
		host = "localhost"
	}
	sendSnapshot := func() error {
		raw, err := marshalProtoJSONList(services.PortsToProto(s.deps.Ports.Snapshot(host, projectID)))
		if err != nil {
			return err
		}
		return w.sendEvent(0, "ports", raw)
	}
	if err := sendSnapshot(); err != nil {
		return
	}
	events := s.deps.Ports.Subscribe()
	defer s.deps.Ports.Unsubscribe(events)
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-events:
			if !ok {
				return
			}
			if err := sendSnapshot(); err != nil {
				return
			}
		}
	}
}

func (s *Server) pumpNotificationsWatch(ctx context.Context, w *streamWriter) {
	events := s.deps.Notifications.Subscribe()
	defer s.deps.Notifications.Unsubscribe(events)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event == nil {
				continue
			}
			if err := w.sendProtoEvent("notification", event); err != nil {
				return
			}
		}
	}
}

func (s *Server) pumpScriptRunWatch(ctx context.Context, w *streamWriter, projectID, runID string) {
	if s.deps.Scripts.GetRun(projectID, runID) == nil {
		_ = w.sendErrorf("Run not found.")
		return
	}
	events, subscribed := s.deps.Scripts.Subscribe(projectID, runID)
	if !subscribed {
		return
	}
	defer s.deps.Scripts.Unsubscribe(projectID, runID, events)
	sendEvent := func(event types.ScriptRunEvent) error {
		msg, name := services.ScriptRunEventToProto(event)
		raw, err := wtProtoMarshal.Marshal(msg)
		if err != nil {
			return err
		}
		return w.sendEvent(0, name, raw)
	}
	// End the stream when the run stops running, detected by a 100ms
	// status poll — mirroring the SSE route, including the final drain so
	// the client always sees the terminal status before the stream ends.
	poll := time.NewTicker(scriptRunPollInterval)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := sendEvent(event); err != nil {
				return
			}
		case <-poll.C:
			if !s.deps.Scripts.IsRunning(projectID, runID) {
				for {
					select {
					case <-ctx.Done():
						return
					case event, ok := <-events:
						if !ok {
							return
						}
						if err := sendEvent(event); err != nil {
							return
						}
					default:
						return
					}
				}
			}
		}
	}
}
