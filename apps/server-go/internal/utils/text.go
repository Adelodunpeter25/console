// Text helpers shared by the title pipeline and the session store. Kept in
// utils (a leaf package) because agent/titles and services/session both need
// them and agent/titles cannot be imported from services/session — agent/loop
// depends on services, so that edge would close an import cycle.
package utils

import (
	"strings"
	"unicode"
)

// CapitalizeFirst uppercases the first cased letter of s and leaves
// everything after it untouched, so "fix login bug" becomes "Fix login bug"
// rather than "FIX LOGIN BUG".
//
// Leading characters that carry no case of their own — emoji, quotes,
// markdown markers, digits, brackets — are skipped, because the first
// *letter* is what a reader expects to see capitalized:
// "🔥 fix login" → "🔥 Fix login", "\"fix login\"" → "\"Fix login\"".
// Returns s unchanged when it holds no cased letter at all.
func CapitalizeFirst(s string) string {
	for i, r := range s {
		if unicode.IsLower(r) {
			return s[:i] + string(unicode.ToUpper(r)) + s[i+len(string(r)):]
		}
		if unicode.IsUpper(r) || unicode.IsTitle(r) {
			return s
		}
	}
	return s
}

// TitleFirst is CapitalizeFirst after a trim: the shape session titles are
// stored in (no surrounding whitespace, first letter capitalized).
func TitleFirst(s string) string {
	return CapitalizeFirst(strings.TrimSpace(s))
}
