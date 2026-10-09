package scratch

import (
	"strings"
)

// TagList splits a comma separated string into clean lowercase tags.
func TagList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.ToLower(strings.TrimSpace(p)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// JoinTags renders tags back into a single comma separated string.
func JoinTags(tags []string) string {
	return strings.Join(tags, ", ")
}
