package cfrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	errNameTaken = errors.New("cfrelay: that relay name is already taken")
	errBadName   = errors.New("cfrelay: that relay name is not valid")
)

// register claims a name (the configured one, or a generated one) and
// stores the identity.
func (m *Manager) register(ctx context.Context, store Store) (*Identity, error) {
	body, _ := json.Marshal(map[string]string{"name": strings.ToLower(strings.TrimSpace(m.Name))})
	if strings.TrimSpace(m.Name) == "" {
		body = []byte("{}")
	}
	endpoint := strings.TrimRight(m.api.String(), "/") + "/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	client := m.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cfrelay: register: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		return nil, errNameTaken
	case http.StatusBadRequest:
		return nil, fmt.Errorf("%w: %s", errBadName, strings.TrimSpace(string(raw)))
	default:
		return nil, fmt.Errorf("cfrelay: register: HTTP %d", res.StatusCode)
	}
	var out struct {
		Name  string `json:"name"`
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Name == "" || out.Token == "" || out.URL == "" {
		return nil, errors.New("cfrelay: register: unexpected response")
	}
	id := Identity{Name: out.Name, URL: out.URL, API: m.api.String(), Token: out.Token}
	if err := store.Save(id); err != nil {
		return nil, err
	}
	m.log().Info("relay name registered", "url", id.URL)
	return &id, nil
}
