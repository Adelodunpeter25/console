// Agent lifecycle notifications. Port of
// apps/server/api/src/services/notify-agent-event.ts: attention banners for
// questions/approvals/errors and done banners with the last assistant
// excerpt. The run service calls NotifyEvent per hub event and NotifyDone
// when a chain settles cleanly.
package run

import (
	"context"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	notificationBodyMax     = 180
	notificationSubtitleMax = 80
)

// TruncateNotificationText collapses whitespace and truncates with ellipsis.
func TruncateNotificationText(value string, max int) string {
	single := strings.Join(strings.Fields(value), " ")
	if len([]rune(single)) <= max {
		return single
	}
	// TS slices UTF-16 units; runes are the closest Go equivalent.
	return strings.TrimRight(string([]rune(single)[:max-1]), " \t") + "…"
}

func cleanSubtitle(title string) string {
	trimmed := strings.Join(strings.Fields(title), " ")
	if trimmed == "" {
		return ""
	}
	return TruncateNotificationText(trimmed, notificationSubtitleMax)
}

// IsAttentionKind reports events that need a "needs attention" banner.
func IsAttentionKind(kind loop.EventKind) bool {
	switch kind {
	case loop.EventAskQuestion, loop.EventPermissionRequest, loop.EventError:
		return true
	default:
		return false
	}
}

// AttentionNotification builds the needs-attention banner for an event.
func AttentionNotification(sessionID string, event loop.Event, sessionTitle string) types.NotificationEvent {
	out := types.NotificationEvent{
		Type: "notification", Kind: "needs_attention", SessionID: sessionID, Title: "Needs Attention",
	}
	if subtitle := cleanSubtitle(sessionTitle); subtitle != "" {
		out.Subtitle = subtitle
	}
	switch event.Kind {
	case loop.EventAskQuestion:
		body := ""
		if event.Ask != nil {
			body = event.Ask.Question
		}
		out.Body = TruncateNotificationText(body, notificationBodyMax)
	case loop.EventPermissionRequest:
		body := ""
		if event.Permission != nil {
			body = event.Permission.ToolName + " is requesting permission"
		}
		out.Body = TruncateNotificationText(body, notificationBodyMax)
	default:
		out.Title = "Agent Error"
		out.Body = TruncateNotificationText(event.Text, notificationBodyMax)
	}
	return out
}

// DoneNotification builds the clean-completion banner with an excerpt.
func DoneNotification(sessionID, sessionTitle, summary string) types.NotificationEvent {
	out := types.NotificationEvent{
		Type: "notification", Kind: "done", SessionID: sessionID, Title: "Done",
	}
	if subtitle := cleanSubtitle(sessionTitle); subtitle != "" {
		out.Subtitle = subtitle
	}
	if summary = strings.Join(strings.Fields(summary), " "); summary != "" {
		out.Body = TruncateNotificationText(summary, notificationBodyMax)
	} else {
		out.Body = "Agent finished"
	}
	return out
}

// notifyEvent pushes an attention banner for questions, approvals, and
// errors (never for aborted runs).
func (s *Service) notifyEvent(ctx context.Context, sessionID string, event loop.Event) {
	bus := s.notifier()
	if bus == nil || !IsAttentionKind(event.Kind) || ctx.Err() != nil {
		return
	}
	title := ""
	if loaded, err := s.sessions.Load(sessionID, 0, 0); err == nil && loaded != nil {
		title = loaded.Header.Title
	}
	bus.Push(AttentionNotification(sessionID, event, title))
}

// notifyDone pushes the clean-completion banner with an excerpt.
func (s *Service) notifyDone(sessionID string) {
	bus := s.notifier()
	if bus == nil {
		return
	}
	title := ""
	if loaded, err := s.sessions.Load(sessionID, 0, 0); err == nil && loaded != nil {
		title = loaded.Header.Title
	}
	bus.Push(DoneNotification(sessionID, title, lastAssistantExcerpt(s.sessions, sessionID)))
}

// lastAssistantExcerpt returns the latest assistant text turn for
// done-banner previews.
func lastAssistantExcerpt(sessions *services.SessionService, sessionID string) string {
	loaded, err := sessions.Load(sessionID, 0, 0)
	if err != nil || loaded == nil {
		return ""
	}
	for i := len(loaded.Messages) - 1; i >= 0; i-- {
		msg, ok := decodeAssistantText(loaded.Messages[i].Data)
		if ok && msg != "" {
			return msg
		}
	}
	return ""
}
