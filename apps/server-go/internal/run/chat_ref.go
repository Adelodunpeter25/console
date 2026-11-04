package run

import (
	"fmt"
	"strings"
)

// chatRefSetup resolves the $refs in a prompt and renders the
// <referenced_chats> block for setup (sent every turn, never persisted,
// so the stored prompt keeps exactly what the user typed). Resolved
// refs name the target chat and point at the read_chat tool; an
// unresolved ref is reported so the model tells the user instead of
// guessing. The current chat is never a valid target.
func (s *Service) chatRefSetup(sessionID, text string) string {
	if s.sessions == nil {
		return ""
	}
	refs := s.sessions.ParseDollarRefs(text)
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<referenced_chats>\n")
	b.WriteString("The user referenced other chats with $ref syntax. Fetch their transcripts with the read_chat tool before answering; do not assume their contents.\n")
	for _, ref := range refs {
		header, err := s.sessions.ResolveReference(sessionID, ref)
		if err != nil {
			fmt.Fprintf(&b, "- $%s: could not be resolved (%v). Tell the user and continue without it.\n", ref, err)
			continue
		}
		fmt.Fprintf(&b, "- $%s -> %q (%s): call read_chat with ref=%q\n", ref, header.Title, header.ID, header.ID)
	}
	b.WriteString("</referenced_chats>")
	return b.String()
}
