package session

import (
	"encoding/json"
	"strings"
)

// PromptEntry is one user prompt in a chat, for the "jump to prompt" lists.
// ID is the message id clients already hold on loaded messages.
type PromptEntry struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"createdAt"`
	Preview   string `json:"preview"`
}

const promptPreviewRunes = 160

// ListPrompts returns every user prompt of a chat, oldest first, without
// loading the rest of the transcript. Server-injected (synthetic) turns are
// left out, as they are everywhere clients render messages. Unknown sessions
// return nil.
func (s *Service) ListPrompts(sessionID string) ([]PromptEntry, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil || !ok {
		return nil, err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	s.ensureBackfilled(sessionID, projectID, conn)

	rows, err := conn.Query(`SELECT id, created_at, content FROM messages WHERE role = 'user' ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prompts := []PromptEntry{}
	for rows.Next() {
		var id, content string
		var createdAt int64
		if err := rows.Scan(&id, &createdAt, &content); err != nil {
			return nil, err
		}
		var msg struct {
			User *struct {
				Content   string `json:"content"`
				Synthetic bool   `json:"synthetic"`
			} `json:"user"`
		}
		if json.Unmarshal([]byte(content), &msg) != nil || msg.User == nil || msg.User.Synthetic {
			continue
		}
		prompts = append(prompts, PromptEntry{ID: id, CreatedAt: createdAt, Preview: promptPreview(msg.User.Content)})
	}
	return prompts, rows.Err()
}

const (
	fileCommentOpen  = `<file_comment path="`
	fileCommentClose = `</file_comment>`
)

// promptPreview is the first non-blank line of the user's own words: staged
// file-comment blocks (the tagged markup clients paint as chips) are dropped,
// and an all-attachment prompt gets a placeholder.
func promptPreview(content string) string {
	text := stripFileComments(content)
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > promptPreviewRunes {
				return string(r[:promptPreviewRunes])
			}
			return line
		}
	}
	return "(attachment)"
}

// stripFileComments mirrors the clients' split_file_comments: an unterminated
// block ends the scan and stays in the text.
func stripFileComments(content string) string {
	var b strings.Builder
	rest := content
	for {
		start := strings.Index(rest, fileCommentOpen)
		if start < 0 {
			break
		}
		afterOpen := rest[start+len(fileCommentOpen):]
		_, tail, ok := strings.Cut(afterOpen, `" lines="`)
		if !ok {
			break
		}
		_, tail, ok = strings.Cut(tail, `">`)
		if !ok {
			break
		}
		_, tail, ok = strings.Cut(tail, fileCommentClose)
		if !ok {
			break
		}
		b.WriteString(rest[:start])
		rest = tail
	}
	b.WriteString(rest)
	return b.String()
}
