// Cross-chat references: $title-slug / $<session-id> resolution.
//
// A reference is a pointer, never payload — the same rule file mentions
// follow. Resolution only names the target chat; the agent pulls its
// transcript on demand with the read_chat tool, so no referenced content
// is inlined into a prompt or persisted into the referencing chat.
//
// Scope is the current chat's project by default (project_id, falling
// back to cwd when the session carries no project). The current chat
// itself is never referenceable.
package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	// DefaultRefLimit / MaxRefLimit bound how many messages one
	// read_chat call returns (most recent first).
	DefaultRefLimit = 50
	MaxRefLimit     = 200
	// refTranscriptCap bounds the rendered transcript in characters so a
	// long referenced chat cannot flood the referencing run.
	refTranscriptCap = 32000
)

// ParseDollarRefs extracts $ref tokens from prompt text, in order,
// deduplicated. A ref is a single slug/id token ($my-chat, $<uuid>)
// preceded by start-of-text or whitespace/opening punctuation, so prices
// ($5), emails and URLs do not count. A token starting with a digit only
// counts when it looks like an id (contains '-' and is long enough).
func ParseDollarRefs(text string) []string {
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(text); i++ {
		if text[i] != '$' {
			continue
		}
		if i > 0 {
			prev := text[i-1]
			if !unicode.IsSpace(rune(prev)) && !strings.ContainsRune("([{\"'<", rune(prev)) {
				continue
			}
		}
		j := i + 1
		for j < len(text) && (isRefChar(text[j])) {
			j++
		}
		token := strings.Trim(text[i+1:j], "-_")
		i = j - 1
		if !isRefToken(token) || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func isRefChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

func isRefToken(token string) bool {
	if token == "" {
		return false
	}
	if token[0] >= '0' && token[0] <= '9' {
		// Prices and plain numbers are not refs; a digit-led token
		// only counts when it looks like a session id.
		return strings.Contains(token, "-") && len(token) >= 8
	}
	return true
}

// SlugifyTitle renders a chat title as the slug a $ref matches:
// lowercase, every run of non-letters/digits collapsed to one '-'.
func SlugifyTitle(title string) string {
	var b strings.Builder
	lastDash := true // trims leading separators
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ResolveReference finds the chat a $ref names, scoped to the current
// chat's project and excluding the current chat itself. Matching is
// tiered: exact id, exact title, title slug, then fuzzy prefix/substring
// on title slug and id. An ambiguous tier is an error listing the
// candidates, so the caller can ask for the full session id.
func (s *Service) ResolveReference(currentSessionID, ref string) (*types.SessionHeader, error) {
	ref = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "$"))
	if ref == "" {
		return nil, fmt.Errorf("empty chat reference")
	}
	current, err := s.Header(currentSessionID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("current chat %q not found", currentSessionID)
	}
	var candidates []types.SessionHeader
	switch {
	case current.ProjectID != nil && *current.ProjectID != "":
		candidates, err = s.ListFiltered(ListFilter{ProjectID: *current.ProjectID})
	case current.Cwd != "":
		candidates, err = s.ListFiltered(ListFilter{Cwd: current.Cwd})
	default:
		return nil, fmt.Errorf("current chat has no project scope to resolve %q in", ref)
	}
	if err != nil {
		return nil, err
	}
	pool := make([]types.SessionHeader, 0, len(candidates))
	for _, h := range candidates {
		if h.ID != currentSessionID {
			pool = append(pool, h)
		}
	}
	lower := strings.ToLower(ref)
	slug := SlugifyTitle(ref)
	if slug == "" {
		slug = lower
	}
	tiers := []func(types.SessionHeader) bool{
		func(h types.SessionHeader) bool { return strings.EqualFold(h.ID, ref) },
		func(h types.SessionHeader) bool { return strings.EqualFold(h.Title, ref) },
		func(h types.SessionHeader) bool { return SlugifyTitle(h.Title) == slug },
		func(h types.SessionHeader) bool {
			return strings.HasPrefix(SlugifyTitle(h.Title), slug+"-") ||
				strings.HasPrefix(strings.ToLower(h.ID), lower)
		},
		func(h types.SessionHeader) bool {
			return strings.Contains(SlugifyTitle(h.Title), slug) ||
				strings.Contains(strings.ToLower(h.Title), lower)
		},
	}
	for _, match := range tiers {
		var hits []types.SessionHeader
		for _, h := range pool {
			if match(h) {
				hits = append(hits, h)
			}
		}
		switch len(hits) {
		case 1:
			return &hits[0], nil
		case 0:
			continue
		default:
			names := make([]string, 0, len(hits))
			for _, h := range hits {
				names = append(names, fmt.Sprintf("%q (%s)", h.Title, h.ID))
			}
			return nil, fmt.Errorf("%q is ambiguous: matches %s — use the full session id", ref, strings.Join(names, ", "))
		}
	}
	return nil, fmt.Errorf("no chat matching %q in this project", ref)
}

// ReadReferencedChat resolves ref and renders the target chat's
// transcript (user and assistant words only: thinking, tool output and
// server-injected synthetic turns are never part of what people said).
// It returns the most recent limit messages, oldest first. before is the
// nextCursor of a previous page (0 = start from the newest); nextCursor
// is 0 when no earlier messages remain.
func (s *Service) ReadReferencedChat(currentSessionID, ref string, limit int, before int64) (title string, sessionID string, transcript string, nextCursor int64, err error) {
	if limit <= 0 {
		limit = DefaultRefLimit
	}
	if limit > MaxRefLimit {
		limit = MaxRefLimit
	}
	if before < 0 {
		return "", "", "", 0, fmt.Errorf("before must be >= 0, got %d", before)
	}
	header, err := s.ResolveReference(currentSessionID, ref)
	if err != nil {
		return "", "", "", 0, err
	}
	loaded, err := s.Load(header.ID, int64(limit), before)
	if err != nil {
		return "", "", "", 0, err
	}
	if loaded == nil {
		return "", "", "", 0, fmt.Errorf("chat %q (%s) not found", header.Title, header.ID)
	}
	cursor := int64(0)
	if loaded.HasMore && loaded.NextCursor != nil {
		cursor = *loaded.NextCursor
	}
	return header.Title, header.ID, renderTranscript(loaded.Messages), cursor, nil
}

// renderTranscript formats stored messages as "User: ...\nAssistant: ..."
// lines. Assistant tool calls are one line each (name only — arguments
// and results stay out), everything else is skipped.
func renderTranscript(messages []json.RawMessage) string {
	var lines []string
	for _, raw := range messages {
		var msg struct {
			User *struct {
				Content   string `json:"content"`
				Synthetic bool   `json:"synthetic"`
			} `json:"user"`
			Assistant *struct {
				Content []json.RawMessage `json:"content"`
			} `json:"assistant"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch {
		case msg.User != nil:
			if msg.User.Synthetic || strings.TrimSpace(msg.User.Content) == "" {
				continue
			}
			lines = append(lines, "User: "+strings.TrimSpace(msg.User.Content))
		case msg.Assistant != nil:
			for _, part := range msg.Assistant.Content {
				var holder map[string]json.RawMessage
				if err := json.Unmarshal(part, &holder); err != nil {
					continue
				}
				if textRaw, ok := holder["text"]; ok {
					var text struct {
						Text string `json:"text"`
					}
					if err := json.Unmarshal(textRaw, &text); err == nil && strings.TrimSpace(text.Text) != "" {
						lines = append(lines, "Assistant: "+strings.TrimSpace(text.Text))
					}
				} else if callRaw, ok := holder["toolCall"]; ok {
					var call struct {
						Name string `json:"name"`
					}
					if err := json.Unmarshal(callRaw, &call); err == nil && call.Name != "" {
						lines = append(lines, fmt.Sprintf("Assistant called %s", call.Name))
					}
				}
			}
		}
	}
	out := strings.Join(lines, "\n")
	if len([]rune(out)) > refTranscriptCap {
		runes := []rune(out)
		out = "[earlier messages omitted — showing the most recent part]\n" + string(runes[len(runes)-refTranscriptCap:])
	}
	return out
}
