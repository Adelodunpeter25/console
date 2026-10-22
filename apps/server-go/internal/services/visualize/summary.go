package visualize

import (
	"fmt"
	"strings"
)

// Summary is the model-facing result. It names the artifact and its contents
// but never includes the spec: the provider sees this text, clients see the
// visual.
func Summary(spec *Spec) string {
	counts := map[string]int{}
	for i := range spec.Blocks {
		counts[spec.Blocks[i].Kind]++
	}
	parts := make([]string, 0, 4)
	if n := counts["metric"]; n > 0 {
		parts = append(parts, plural(n, "metric"))
	}
	if n := counts["chart"]; n > 0 {
		parts = append(parts, plural(n, "chart"))
	}
	if n := counts["table"]; n > 0 {
		parts = append(parts, plural(n, "table"))
	}
	if n := counts["text"]; n > 0 {
		parts = append(parts, plural(n, "note"))
	}
	body := strings.Join(parts, ", ")
	if spec.Title != "" {
		return fmt.Sprintf("Rendered visual %q (%s).", spec.Title, body)
	}
	return fmt.Sprintf("Rendered visual (%s).", body)
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
