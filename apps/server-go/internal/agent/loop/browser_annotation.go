// Browser annotation rendering: turns the structured annotations attached to
// a user message into the <browser_annotation> blocks the model reads.
//
// Formatting lives here rather than in message.go because it is pure string
// building with its own escaping and size limits, and because both the
// materialize path and the token estimators need the rendered text.
package loop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Size caps. The inspector already truncates the HTML snippet, but a client
// can send anything, so the renderer bounds every free-form field. Styles are
// capped by count and value length; the comment is capped because it is the
// one field a user can type an essay into.
const (
	maxSnippetChars  = 1000
	maxCommentChars  = 2000
	maxStyleEntries  = 40
	maxStyleValChars = 120
	maxTextField     = 300
)

// renderAnnotations formats annotations as one <browser_annotation> block per
// element. Returns "" when there is nothing worth sending, so callers can
// append the result unconditionally.
func renderAnnotations(annotations []types.BrowserAnnotation) string {
	if len(annotations) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(annotations))
	for _, a := range annotations {
		if block := renderAnnotation(a); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return "\n" + strings.Join(blocks, "\n")
}

func renderAnnotation(a types.BrowserAnnotation) string {
	var b strings.Builder
	b.WriteString("<browser_annotation>\n")

	writeTag(&b, "target_url", a.URL)
	writeTag(&b, "page_title", a.Title)
	writeTag(&b, "component", a.ComponentName)
	writeTag(&b, "source_location", a.SourceLocation)
	writeTag(&b, "css_selector", a.Selector)
	writeTag(&b, "accessibility", accessibilitySummary(a.Role, a.AccessibleName))
	if dims := a.Dimensions; dims != nil {
		writeTag(&b, "element_dimensions", fmt.Sprintf(
			"%s x %s at (x: %s, y: %s)",
			formatPx(dims.Width), formatPx(dims.Height), formatPx(dims.X), formatPx(dims.Y),
		))
	}
	if styles := formatStyles(a.ComputedStyles); styles != "" {
		writeTag(&b, "computed_styles", styles)
	}
	if snippet := truncate(strings.TrimSpace(a.HTMLSnippet), maxSnippetChars); snippet != "" {
		b.WriteString("  <html_snippet><![CDATA[" + cdataSafe(snippet) + "]]></html_snippet>\n")
	}
	if comment := truncate(strings.TrimSpace(a.UserComment), maxCommentChars); comment != "" {
		writeTag(&b, "user_annotation", comment)
	}

	b.WriteString("</browser_annotation>")
	return b.String()
}

// writeTag emits an indented element, or nothing when the value is empty —
// absent metadata should not show up as an empty tag the model has to
// interpret.
func writeTag(b *strings.Builder, tag, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString("  <" + tag + ">" + escapeXML(value) + "</" + tag + ">\n")
}

func accessibilitySummary(role, name string) string {
	role = strings.TrimSpace(role)
	name = truncate(strings.TrimSpace(name), maxTextField)
	switch {
	case role != "" && name != "":
		return role + " / " + name
	case role != "":
		return role
	default:
		return name
	}
}

// formatStyles renders the computed-style map in a stable order (sorted by
// property) so identical elements always produce identical prompt text, which
// keeps provider prompt caching effective.
func formatStyles(styles map[string]string) string {
	if len(styles) == 0 {
		return ""
	}
	props := make([]string, 0, len(styles))
	for prop := range styles {
		props = append(props, prop)
	}
	sort.Strings(props)

	parts := make([]string, 0, len(props))
	for _, prop := range props {
		value := strings.TrimSpace(styles[prop])
		if value == "" {
			continue
		}
		parts = append(parts, prop+": "+truncate(value, maxStyleValChars))
		if len(parts) == maxStyleEntries {
			break
		}
	}
	return strings.Join(parts, "; ")
}

// formatPx rounds to whole pixels and drops the decimal, since layout values
// are only meaningful at that precision for a human-readable prompt.
func formatPx(v float64) string {
	return fmt.Sprintf("%.0f", v)
}

// cdataSafe neutralises a "]]>" sequence that would otherwise terminate the
// CDATA section early and inject markup into the prompt.
func cdataSafe(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

func escapeXML(s string) string { return xmlEscaper.Replace(s) }

// truncate caps s at limit runes, appending an ellipsis marker when cut.
func truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

// AnnotationChars measures the rendered annotation markup for a set of
// annotations. Exported for the compaction token estimators, which must count
// what the materializer actually appends to the message content.
func AnnotationChars(annotations []types.BrowserAnnotation) int {
	return annotationChars(annotations)
}

// annotationChars measures the rendered annotation markup for a message. Image
// bytes carried on an annotation are counted by the image accounting, so only
// the text is measured here.
func annotationChars(annotations []types.BrowserAnnotation) int {
	return len([]rune(renderAnnotations(annotations)))
}
