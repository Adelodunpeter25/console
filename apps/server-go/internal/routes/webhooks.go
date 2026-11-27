// Public webhook ingress (/api/webhooks/:topic and /api/webhooks/:topic/:id).
// Registered above RequireDeviceToken because external senders authenticate
// with the subscription's secret or HMAC signature rather than a device token.
package routes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// WebhookDeliverFunc matches and delivers an incoming webhook payload.
type WebhookDeliverFunc func(topic, subID string, headers map[string]string, queryToken string, body []byte) (int, error)

// RegisterWebhookRoutes wires the public webhook ingress endpoints.
func RegisterWebhookRoutes(app *fiber.App, deliver WebhookDeliverFunc) {
	h := app.Group("/api/webhooks")
	handler := func(c *fiber.Ctx) error {
		if deliver == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "error": "webhooks unavailable"})
		}
		topic := c.Params("topic")
		subID := c.Params("id")
		headers := make(map[string]string)
		c.Request().Header.VisitAll(func(k, v []byte) {
			headers[string(k)] = string(v)
		})
		queryToken := c.Query("token")
		matched, err := deliver(topic, subID, headers, queryToken, c.Body())
		switch {
		case err == nil:
			return c.JSON(fiber.Map{"success": true, "matched": matched})
		case errors.Is(err, webhooks.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": err.Error()})
		case errors.Is(err, webhooks.ErrUnauthorized):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
	}
	h.Post("/:topic", handler)
	h.Post("/:topic/:id", handler)
}

type webhookView struct {
	webhooks.Subscription
	WebhookPath         string `json:"webhookPath"`
	TopicPath           string `json:"topicPath"`
	WebhookURL          string `json:"webhookUrl,omitempty"`
	WebhookURLWithToken string `json:"webhookUrlWithToken,omitempty"`
}

func toWebhookView(sub webhooks.Subscription, baseURL string) webhookView {
	path := fmt.Sprintf("/api/webhooks/%s/%s", sub.Topic, sub.ID)
	view := webhookView{
		Subscription: sub,
		WebhookPath:  path,
		TopicPath:    fmt.Sprintf("/api/webhooks/%s", sub.Topic),
	}
	if base := strings.TrimRight(strings.TrimSpace(baseURL), "/"); base != "" {
		view.WebhookURL = base + path
		view.WebhookURLWithToken = base + path + "?token=" + sub.Secret
	}
	return view
}

// RegisterWebhookManagementRoutes wires the authenticated webhook management
// and SSE endpoints onto app.
func RegisterWebhookManagementRoutes(app *fiber.App, store *webhooks.Service, baseURLFn func() string) {
	if baseURLFn == nil {
		baseURLFn = defaultWebhookBaseURL
	}
	h := app.Group("/api/webhooks")

	h.Get("/", func(c *fiber.Ctx) error {
		sessionID := c.Query("sessionId")
		var (
			list []webhooks.Subscription
			err  error
		)
		if sessionID != "" {
			list, err = store.ListForSession(sessionID)
		} else {
			list, err = store.ListAll()
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		base := baseURLFn()
		views := make([]webhookView, 0, len(list))
		for _, sub := range list {
			views = append(views, toWebhookView(sub, base))
		}
		return c.JSON(fiber.Map{"success": true, "data": views})
	})

	h.Get("/stream", func(c *fiber.Ctx) error {
		sessionID := c.Query("sessionId")
		return streamSSE(c, func(sse *sseStream) {
			events := store.Subscribe()
			defer store.Unsubscribe(events)

			snapshotOf := func() string {
				var (
					list []webhooks.Subscription
					err  error
				)
				if sessionID != "" {
					list, err = store.ListForSession(sessionID)
				} else {
					list, err = store.ListAll()
				}
				if err != nil {
					return "[]"
				}
				base := baseURLFn()
				views := make([]webhookView, 0, len(list))
				for _, sub := range list {
					views = append(views, toWebhookView(sub, base))
				}
				return mustJSON(views)
			}

			if err := sse.Send("webhooks", snapshotOf()); err != nil {
				return
			}
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case ev := <-events:
					if sessionID != "" && ev.SessionID != "" && ev.SessionID != sessionID {
						continue
					}
					if err := sse.Send("webhook", mustJSON(ev)); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})

	h.Post("/", func(c *fiber.Ctx) error {
		var req struct {
			SessionID        string         `json:"sessionId"`
			Topic            string         `json:"topic"`
			Prompt           string         `json:"prompt"`
			Secret           string         `json:"secret"`
			Filter           map[string]any `json:"filter"`
			Once             *bool          `json:"once"`
			ExpiresInSeconds int64          `json:"expiresInSeconds"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "invalid request body"})
		}
		once := true
		if req.Once != nil {
			once = *req.Once
		}
		now := time.Now()
		var expiresAt int64
		if req.ExpiresInSeconds > 0 {
			expiresAt = now.Add(time.Duration(req.ExpiresInSeconds) * time.Second).Unix()
		}
		sub, err := store.Create(req.SessionID, req.Topic, req.Prompt, req.Secret, req.Filter, once, expiresAt, now)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": toWebhookView(*sub, baseURLFn())})
	})

	h.Get("/:id", func(c *fiber.Ctx) error {
		sub, err := store.Get(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if sub == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "webhook subscription not found"})
		}
		return c.JSON(fiber.Map{"success": true, "data": toWebhookView(*sub, baseURLFn())})
	})

	h.Delete("/:id", func(c *fiber.Ctx) error {
		deleted, err := store.Delete(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if !deleted {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "webhook subscription not found"})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"deleted": true}})
	})
}

func defaultWebhookBaseURL() string {
	return webhookBaseURLFromDir(filepath.Join(utils.ConsoleStorageDir(), "tunnel"))
}

func webhookBaseURLFromDir(tunnelDir string) string {
	if explicit := strings.TrimSpace(os.Getenv("CONSOLE_PUBLIC_URL")); explicit != "" {
		return strings.TrimRight(explicit, "/")
	}
	if info, ok := opentunnel.ReadInfo(tunnelDir); ok && info.Ready && info.URL != "" {
		return strings.TrimRight(info.URL, "/")
	}
	if remote, ok := tunnel.ReadInfo(tunnelDir); ok && remote.Ready && remote.URL != "" {
		return strings.TrimRight(remote.URL, "/")
	}
	return ""
}
