// Minimal frontmatter parser (no YAML dependency). Port of
// apps/server/agent/src/systemprompt/frontmatter.ts.
package systemprompt

import (
	"strings"
)

// Frontmatter is the parsed key/value block plus the body that follows it.
type Frontmatter struct {
	Values map[string]string
	Body   string
}

func kebabToCamel(key string) string {
	if !strings.Contains(key, "-") {
		return key
	}
	var b strings.Builder
	upperNext := false
	for _, r := range key {
		if r == '-' {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteString(strings.ToUpper(string(r)))
			upperNext = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// ParseFrontmatter parses an optional `---`-delimited block at the start of
// content. Returns an empty Frontmatter (with the original body) when none
// is present. Values are stored as raw strings; helpers below coerce them.
func ParseFrontmatter(content string) Frontmatter {
	text := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(text, "---") {
		return Frontmatter{Values: map[string]string{}, Body: text}
	}
	end := strings.Index(text[3:], "\n---")
	if end == -1 {
		return Frontmatter{Values: map[string]string{}, Body: text}
	}
	end += 3
	rawMeta := strings.TrimPrefix(text[3:end], "\r\n")
	rawMeta = strings.TrimPrefix(rawMeta, "\n")
	body := text[end+4:]
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")

	values := map[string]string{}
	for _, line := range strings.Split(rawMeta, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}
		key := kebabToCamel(strings.TrimSpace(trimmed[:colon]))
		value := unquote(strings.TrimSpace(trimmed[colon+1:]))
		values[key] = value
	}
	return Frontmatter{Values: values, Body: body}
}

// AsBool coerces a frontmatter value ("true"/"1"/"false"/"0") to bool; ok is
// false when the key is absent or unrecognized.
func (f Frontmatter) AsBool(key string) (bool, bool) {
	raw, present := f.Values[key]
	if !present {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

// AsStringSlice splits a comma-separated frontmatter value (e.g. globs).
func (f Frontmatter) AsStringSlice(key string) []string {
	raw, present := f.Values[key]
	if !present {
		return nil
	}
	raw = strings.Trim(raw, "[]")
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.Trim(strings.TrimSpace(part), "\"'")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
