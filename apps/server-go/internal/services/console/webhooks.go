// The webhook methods: console.webhooks.* — generic event subscriptions an
// agent creates for its own session. They act on the calling session only;
// incoming HTTP requests on /api/webhooks/:topic(/:id) wake the session.
package console

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
)

func requireWebhooks(r *Runtime) (*webhooks.Service, error) {
	if r.webhooks == nil {
		return nil, fmt.Errorf("webhooks are unavailable")
	}
	return r.webhooks, nil
}

func (r *Runtime) webhookPayload(sub *webhooks.Subscription) map[string]any {
	path := fmt.Sprintf("/api/webhooks/%s/%s", sub.Topic, sub.ID)
	out := map[string]any{
		"id":            sub.ID,
		"session_id":    sub.SessionID,
		"topic":         sub.Topic,
		"secret":        sub.Secret,
		"prompt":        sub.Prompt,
		"once":          sub.Once,
		"status":        sub.Status,
		"fire_count":    sub.FireCount,
		"created_at":    sub.CreatedAt,
		"now":           time.Now().Unix(),
		"webhook_path":  path,
		"topic_path":    fmt.Sprintf("/api/webhooks/%s", sub.Topic),
	}
	if len(sub.Filter) > 0 {
		out["filter"] = sub.Filter
	}
	if sub.ExpiresAt > 0 {
		out["expires_at"] = sub.ExpiresAt
	}
	if sub.LastFiredAt > 0 {
		out["last_fired_at"] = sub.LastFiredAt
	}
	if sub.LastError != "" {
		out["last_error"] = sub.LastError
	}
	if r.webhookBaseURL != nil {
		if base := strings.TrimRight(strings.TrimSpace(r.webhookBaseURL()), "/"); base != "" {
			out["webhook_url"] = base + path
			out["webhook_url_with_token"] = base + path + "?token=" + sub.Secret
		}
	}
	return out
}

func optionalBool(args map[string]any, name string, fallback bool) (bool, error) {
	value, present := args[name]
	if !present {
		return fallback, nil
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean, got %v", name, value)
	}
	return b, nil
}

func optionalObject(args map[string]any, name string) (map[string]any, error) {
	value, present := args[name]
	if !present || value == nil {
		return nil, nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object, got %v", name, value)
	}
	return obj, nil
}

// webhooksSubscribe creates a generic webhook subscription for this session.
func (r *Runtime) webhooksSubscribe(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireWebhooks(r)
	if err != nil {
		return nil, err
	}
	topic, err := stringArg(args, "topic")
	if err != nil {
		return nil, err
	}
	prompt, err := stringArg(args, "prompt")
	if err != nil {
		return nil, err
	}
	filter, err := optionalObject(args, "filter")
	if err != nil {
		return nil, err
	}
	once, err := optionalBool(args, "once", true)
	if err != nil {
		return nil, err
	}
	secret, err := optionalString(args, "secret")
	if err != nil {
		return nil, err
	}
	expiresIn, hasExpires, err := optionalInt(args, "expires_in_seconds")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var expiresAt int64
	if hasExpires {
		if expiresIn <= 0 {
			return nil, fmt.Errorf("expires_in_seconds must be positive")
		}
		expiresAt = now.Add(time.Duration(expiresIn) * time.Second).Unix()
	}
	sub, err := store.Create(r.sessionID, topic, prompt, secret, filter, once, expiresAt, now)
	if err != nil {
		return nil, err
	}
	return marshalResult(r.webhookPayload(sub))
}

// webhooksList lists every webhook subscription belonging to this session.
func (r *Runtime) webhooksList(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	store, err := requireWebhooks(r)
	if err != nil {
		return nil, err
	}
	list, err := store.ListForSession(r.sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list))
	for i := range list {
		out = append(out, r.webhookPayload(&list[i]))
	}
	return marshalResult(out)
}

// webhooksGet reads one webhook subscription belonging to this session.
func (r *Runtime) webhooksGet(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireWebhooks(r)
	if err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	sub, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.SessionID != r.sessionID {
		return nil, fmt.Errorf("no webhook subscription %q on this session", id)
	}
	return marshalResult(r.webhookPayload(sub))
}

// webhooksUnsubscribe deletes a webhook subscription belonging to this session.
func (r *Runtime) webhooksUnsubscribe(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireWebhooks(r)
	if err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	sub, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.SessionID != r.sessionID {
		return nil, fmt.Errorf("no webhook subscription %q on this session", id)
	}
	if _, err := store.Delete(id); err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{"id": id, "deleted": true})
}
