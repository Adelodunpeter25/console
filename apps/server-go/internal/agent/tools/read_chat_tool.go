// read_chat tool: fetch the transcript of another chat in the same
// project. The user names it with a $ref ($title-slug or $<session-id>)
// in their prompt; the agent resolves it here, on demand, instead of
// having the referenced transcript inlined into the prompt.
package tools

import (
	"context"
	"fmt"
	"strings"
)

type readChatInput struct {
	Ref   string `json:"ref" jsonschema:"required,description=Referenced chat: its $title-slug or full session id (leading $ optional)"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max messages to return, most recent first (default 50, max 200)"`
}

// ChatReader is the persistence seam the tool reads through — satisfied
// by *services.SessionService (kept as an interface here to avoid tools
// depending on the concrete session storage implementation).
type ChatReader interface {
	ReadReferencedChat(currentSessionID, ref string, limit int) (title string, sessionID string, transcript string, err error)
}

// NewReadChatTool builds read_chat bound to the current session, so a
// chat can never reference itself and can only reach its own project.
// A nil reader reports unavailable (the run service always wires one).
func NewReadChatTool(currentSessionID string, reader ChatReader) Tool {
	return NewTool("read_chat",
		"Read the transcript of another chat in this project, named by a $reference ($title-slug or $<session-id>) the user wrote. Returns the most recent user/assistant messages; thinking and tool output are excluded. Only chats from the same project as this one can be read, and never this chat itself.",
		TierRead,
		func(ctx context.Context, in readChatInput) (any, error) {
			if reader == nil {
				return nil, NewToolError("read_chat is not available in this run.")
			}
			ref := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(in.Ref), "$"))
			if ref == "" {
				return nil, NewToolError("'ref' is required: a $title-slug or session id.")
			}
			title, id, transcript, err := reader.ReadReferencedChat(currentSessionID, ref, in.Limit)
			if err != nil {
				return nil, NewToolError("%s", err.Error())
			}
			if strings.TrimSpace(transcript) == "" {
				return textResult(fmt.Sprintf("Chat %q (%s):\n(No messages yet)", title, id)), nil
			}
			return textResult(fmt.Sprintf("Chat %q (%s):\n\n%s", title, id, transcript)), nil
		})
}
