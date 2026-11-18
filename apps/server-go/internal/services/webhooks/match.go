package webhooks

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Match finds active subscriptions for topic (and optional subID) whose secret
// verifies the request and whose filter matches the JSON payload.
//
// When subID is given:
//   - unknown id or mismatched topic returns ErrNotFound
//   - inactive/expired subscription returns nil, nil (ignored)
//   - failed auth returns ErrUnauthorized
func (s *Service) Match(topic, subID string, headers map[string]string, queryToken string, body []byte, now time.Time) ([]Subscription, error) {
	var candidates []Subscription
	if subID != "" {
		sub, err := s.Get(subID)
		if err != nil {
			return nil, err
		}
		if sub == nil || (topic != "" && sub.Topic != topic) {
			return nil, ErrNotFound
		}
		if sub.Status != StatusActive {
			return nil, nil
		}
		if sub.ExpiresAt > 0 && sub.ExpiresAt <= now.Unix() {
			_, _ = s.SetStatus(sub.ID, StatusExpired)
			return nil, nil
		}
		if !verifyAuth(sub.Secret, headers, queryToken, body) {
			return nil, ErrUnauthorized
		}
		candidates = []Subscription{*sub}
	} else {
		rows, err := s.db.Query(
			`SELECT `+columns+` FROM webhook_subscriptions WHERE topic = ? AND status = 'active' ORDER BY created_at`,
			topic,
		)
		if err != nil {
			return nil, err
		}
		all, err := scanRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		for _, sub := range all {
			if sub.ExpiresAt > 0 && sub.ExpiresAt <= now.Unix() {
				_, _ = s.SetStatus(sub.ID, StatusExpired)
				continue
			}
			if verifyAuth(sub.Secret, headers, queryToken, body) {
				candidates = append(candidates, sub)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	var payload any
	if len(strings.TrimSpace(string(body))) > 0 {
		_ = json.Unmarshal(body, &payload)
	}

	var matched []Subscription
	for _, sub := range candidates {
		if matchFilter(sub.Filter, payload) {
			matched = append(matched, sub)
		}
	}
	return matched, nil
}

func matchFilter(filter map[string]any, payload any) bool {
	if len(filter) == 0 {
		return true
	}
	root, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	for path, expected := range filter {
		actual, found := lookupPath(root, path)
		if !found || !matchValue(expected, actual) {
			return false
		}
	}
	return true
}

func lookupPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

func matchValue(expected, actual any) bool {
	if list, ok := expected.([]any); ok {
		for _, item := range list {
			if scalarEqual(item, actual) {
				return true
			}
		}
		return false
	}
	return scalarEqual(expected, actual)
}

func scalarEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if fa, okA := toFloat(a); okA {
		if fb, okB := toFloat(b); okB {
			return fa == fb
		}
	}
	if ba, okA := a.(bool); okA {
		if bb, okB := b.(bool); okB {
			return ba == bb
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := strconv.ParseFloat(n.String(), 64)
		return f, err == nil
	}
	return 0, false
}
