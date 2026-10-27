// Vision handoff: one non-streaming, tool-free call to a vision model that
// returns prose describing an image for a run model that cannot see it.
// Mirrors internal/agent/titles.
package vision

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
)

const systemPrompt = "You describe images for a text-only coding agent. " +
	"Report, in this order and concisely: what the image shows; any visible text " +
	"verbatim; UI state, dialogs, or error messages; layout and structure; anything " +
	"that looks like a failure. Be factual. Do not speculate, do not apologise, " +
	"do not ask questions, and do not use markdown headings."

// Describe performs one non-streaming, tool-free call to a vision model and
// returns its prose description. Returns "" on any failure so the caller can
// fall back to a placeholder. accountID, when set, pins the call to one
// account (decision 7).
func Describe(ctx context.Context, provider loop.Provider, model, mime string, data []byte, hint, accountID string) string {
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model:        model,
		SystemPrompt: systemPrompt,
		Messages: []any{loop.UserMessage{
			Role:        loop.RoleUser,
			Content:     userPrompt(hint),
			Attachments: []loop.ImageAttachment{{Data: base64.StdEncoding.EncodeToString(data), MimeType: mime}},
		}},
		AccountID: accountID,
		// Tools deliberately nil: the describer must not read another image
		// and recurse.
	}
	if err := provider.RunTurn(ctx, req, events); err != nil {
		return ""
	}
	var out strings.Builder
	for {
		event, err, ok := events.Next()
		if !ok || err != nil {
			break
		}
		if event.Kind == loop.EventText {
			out.WriteString(event.Text)
		}
	}
	return strings.TrimSpace(out.String())
}

func userPrompt(hint string) string {
	if strings.TrimSpace(hint) == "" {
		return "Describe this image."
	}
	return "Describe this image, read from " + hint + "."
}
