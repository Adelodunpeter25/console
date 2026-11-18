// Webhook delivery: wakes sessions whose event subscription matched an
// incoming webhook request. A fire is an ordinary prompt on the owning session
// (start now when idle, queue when busy), so it appears in the thread with
// full context.
package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
)

const webhookPayloadLimit = 16 * 1024

var ErrWebhooksUnavailable = errors.New("webhooks are unavailable")

// SetWebhooks attaches the webhook subscription store backing
// console.webhooks.* and the public webhook ingress route.
func (s *Service) SetWebhooks(store *webhooks.Service) {
	s.mu.Lock()
	s.webhooks = store
	s.mu.Unlock()
}

func (s *Service) webhookStore() *webhooks.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.webhooks
}

// SetWebhookBaseURL attaches a resolver for the server's public webhook base
// URL so console.webhooks.subscribe can return full URLs.
func (s *Service) SetWebhookBaseURL(fn func() string) {
	s.mu.Lock()
	manager := s.consoleManager
	s.mu.Unlock()
	if manager != nil {
		manager.SetWebhookBaseURL(fn)
	}
}

// DeliverWebhook matches an incoming webhook request against active
// subscriptions and wakes each matching session.
func (s *Service) DeliverWebhook(topic, subID string, headers map[string]string, queryToken string, body []byte) (int, error) {
	store := s.webhookStore()
	if store == nil {
		return 0, ErrWebhooksUnavailable
	}
	now := time.Now()
	matched, err := store.Match(topic, subID, headers, queryToken, body, now)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, sub := range matched {
		prompt := formatWebhookPrompt(sub, headers, body)
		var fireErr error
		if _, err := s.promptSession(sub.SessionID, prompt, "", "", "", ""); err != nil {
			if errors.Is(err, ErrNoSession) {
				fireErr = err
			} else {
				slog.Warn("webhook wake failed", "subscription", sub.ID, "session", sub.SessionID, "error", err)
				continue
			}
		}
		if err := store.AfterFire(sub.ID, fireErr, now); err != nil {
			slog.Warn("webhook post-fire update failed", "subscription", sub.ID, "error", err)
		} else if fireErr == nil {
			delivered++
			slog.Info("webhook delivered", "subscription", sub.ID, "session", sub.SessionID, "topic", sub.Topic)
		}
	}
	return delivered, nil
}

func formatWebhookPrompt(sub webhooks.Subscription, headers map[string]string, body []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Webhook event received]\nSubscription: %s (topic: %s)\n", sub.ID, sub.Topic)
	for _, key := range []string{"X-GitHub-Event", "X-Event-Type", "X-Webhook-Event"} {
		for hk, hv := range headers {
			if strings.EqualFold(hk, key) && strings.TrimSpace(hv) != "" {
				fmt.Fprintf(&b, "Event: %s\n", strings.TrimSpace(hv))
				break
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(sub.Prompt)

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 {
		formatted := string(trimmed)
		var pretty bytes.Buffer
		if json.Indent(&pretty, trimmed, "", "  ") == nil {
			formatted = pretty.String()
		}
		if len(formatted) > webhookPayloadLimit {
			formatted = formatted[:webhookPayloadLimit] + "\n... (truncated)"
		}
		b.WriteString("\n\nPayload:\n")
		b.WriteString(formatted)
	}
	return b.String()
}
