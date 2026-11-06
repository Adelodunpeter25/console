package session

import (
	"database/sql"
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
	s.ensureContentDropped(sessionID, projectID, conn)

	// The content column exists only while the stage-6 migration is pending or
	// its audit refused the drop; a literal keeps one query shape for both.
	contentSel := "''"
	if hasContentColumn(conn) {
		contentSel = "content"
	}
	rows, err := conn.Query(`SELECT id, created_at, ` + contentSel + `, skeleton, split_state FROM messages WHERE role = 'user' ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	type promptRow struct {
		id        string
		createdAt int64
		content   sql.NullString
		skeleton  sql.NullString
		state     int
	}
	scan := make([]promptRow, 0)
	for rows.Next() {
		var r promptRow
		if err := rows.Scan(&r.id, &r.createdAt, &r.content, &r.skeleton, &r.state); err != nil {
			return nil, err
		}
		scan = append(scan, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	srows := make([]storedRow, len(scan))
	for i, r := range scan {
		srows[i] = storedRow{id: r.id, content: r.content, skeleton: r.skeleton, state: r.state}
	}
	bodies := s.materialize(sessionID, projectID, conn, srows)
	prompts := []PromptEntry{}
	for i, r := range scan {
		if bodies[i] == "" {
			continue
		}
		var msg struct {
			User *struct {
				Content   string `json:"content"`
				Synthetic bool   `json:"synthetic"`
			} `json:"user"`
		}
		if json.Unmarshal([]byte(bodies[i]), &msg) != nil || msg.User == nil || msg.User.Synthetic {
			continue
		}
		prompts = append(prompts, PromptEntry{ID: r.id, CreatedAt: r.createdAt, Preview: promptPreview(msg.User.Content)})
	}
	return prompts, nil
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
