// Compaction entry points: threshold checks, history rewriting, and
// overflow detection. Port of the exported surface of
// apps/server/agent/src/compaction/index.ts plus the overflow predicate in
// agent/src/utils/error.ts.
package compaction

import (
	"math"
	"regexp"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Options mirrors CompactionOptions. Summarize, when set with
// SummaryStrategy "llm", produces the narrative summary.
type Options struct {
	Enabled            *bool
	MaxThresholdRatio  float64
	KeepRecentTokens   int
	TokenThreshold     int
	MinimumRecentTurns int
	MaxToolResultChars int
	EmergencyChars     int
	SummaryStrategy    string // "structural" (default) or "llm"
	Summarize          func(messages []any) (string, error)
}

// Result mirrors CompactionResult.
type Result struct {
	CompactedMessages []any
	Summary           string
	OriginalCount     int
	TokensBefore      int
	TokensAfter       int
}

func (o Options) enabled() bool {
	return o.Enabled == nil || *o.Enabled
}

func (o Options) keepRecent() int {
	if o.KeepRecentTokens > 0 {
		return o.KeepRecentTokens
	}
	return 40000
}

func (o Options) minRecentTurns() int {
	if o.MinimumRecentTurns > 0 {
		return o.MinimumRecentTurns
	}
	return 3
}

func (o Options) maxToolChars() int {
	if o.MaxToolResultChars > 0 {
		return o.MaxToolResultChars
	}
	return 8000
}

func (o Options) emergencyChars() int {
	if o.EmergencyChars > 0 {
		return o.EmergencyChars
	}
	return EmergencyChars
}

// ShouldCompact reports whether history exceeds the context threshold
// (payload-aware: system prompt + tools + history).
func ShouldCompact(history []any, contextWindow int, options Options, systemPrompt string, defs []tools.Definition) bool {
	if !options.enabled() || contextWindow <= 0 {
		return false
	}
	ratio := options.MaxThresholdRatio
	if ratio <= 0 {
		ratio = 0.85
	}
	tokens := EstimatePayloadTokens(history, systemPrompt, defs).Tokens
	limit := options.TokenThreshold
	if limit <= 0 {
		limit = int(math.Floor(float64(contextWindow) * ratio))
	}
	return tokens >= limit
}

// CompactHistory summarizes older turns into a checkpoint, keeping the
// recent tail intact.
func CompactHistory(history []any, options Options) Result {
	tokensBefore := EstimateMessageTokens(history)
	keepRecent, minRecent, maxToolChars := options.keepRecent(), options.minRecentTurns(), options.maxToolChars()
	var shaken []any
	if len(history) <= 4 {
		shaken = ShakeConversation(history, maxToolChars, len(history), true)
	} else {
		shaken = ShakeConversation(history, maxToolChars, ProtectedRecentStart(history, minRecent), false)
	}
	cut := FindCutPoint(shaken, keepRecent, minRecent)
	if cut.FirstKeptIndex == 0 || len(history) <= 4 {
		tokensAfter := EstimateMessageTokens(shaken)
		if tokensAfter >= tokensBefore {
			return Result{
				CompactedMessages: append([]any{}, history...),
				Summary:           "History too short or cannot be safely partitioned.",
				OriginalCount:     len(history),
				TokensBefore:      tokensBefore,
				TokensAfter:       tokensBefore,
			}
		}
		return Result{
			CompactedMessages: append([]any{}, shaken...),
			Summary:           "History too short or cannot be safely partitioned.",
			OriginalCount:     len(history),
			TokensBefore:      tokensBefore,
			TokensAfter:       tokensAfter,
		}
	}
	older := shaken[:cut.FirstKeptIndex]
	recent := shaken[cut.FirstKeptIndex:]
	summary := BuildStructuralSummary(older)
	summaryUser := loop.UserMessage{Role: loop.RoleUser, Content: summary}
	var compacted []any
	if cut.IsUserBoundary {
		compacted = append([]any{summaryUser, assistantAck()}, recent...)
	} else {
		compacted = append([]any{summaryUser}, recent...)
	}
	tokensAfter := EstimateMessageTokens(compacted)
	if tokensAfter >= tokensBefore {
		shakenTokens := EstimateMessageTokens(shaken)
		if shakenTokens >= tokensBefore {
			return Result{
				CompactedMessages: append([]any{}, history...),
				Summary:           "Compaction skipped: no safe token reduction.",
				OriginalCount:     len(history),
				TokensBefore:      tokensBefore,
				TokensAfter:       tokensBefore,
			}
		}
		return Result{
			CompactedMessages: append([]any{}, shaken...),
			Summary:           "Compaction summary skipped: structural summary would not reduce context.",
			OriginalCount:     len(history),
			TokensBefore:      tokensBefore,
			TokensAfter:       shakenTokens,
		}
	}
	return Result{
		CompactedMessages: compacted,
		Summary:           summary,
		OriginalCount:     len(history),
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
	}
}

func assistantAck() loop.AssistantMessage {
	return loop.AssistantMessage{
		Role: loop.RoleAssistant,
		Content: []any{loop.TextPart{
			Type: "text",
			Text: "Understood. I have the context of prior work and files touched. Ready to proceed.",
		}},
		StopReason: loop.StopStop,
	}
}

// CompactHistoryWithSummary rewrites history around an externally produced
// (e.g. LLM) summary, mirroring compactHistoryWithSummary.
func CompactHistoryWithSummary(history []any, options Options, summary string) Result {
	structural := CompactHistory(history, options)
	if len(structural.CompactedMessages) == len(history) {
		return structural
	}
	firstUserIndex := -1
	for i, m := range structural.CompactedMessages {
		if isUserMessage(m) {
			firstUserIndex = i
			break
		}
	}
	if firstUserIndex < 0 {
		return structural
	}
	rest := structural.CompactedMessages[firstUserIndex+1:]
	compacted := append([]any{loop.UserMessage{Role: loop.RoleUser, Content: summary}}, rest...)
	return Result{
		CompactedMessages: compacted,
		Summary:           summary,
		OriginalCount:     len(history),
		TokensBefore:      structural.TokensBefore,
		TokensAfter:       EstimateMessageTokens(compacted),
	}
}

// overflowPatterns mirrors OVERFLOW_MESSAGE_PATTERNS across providers.
var overflowPatterns = []string{
	"input token count exceeds",
	"context_length_exceeded",
	"prompt_too_long",
	"maximum context length",
	"context window",
	"context limit",
	"too many tokens",
}

// IsContextOverflowError reports provider failures meaning the request
// exceeded the context window (recover with emergency compaction + retry).
func IsContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, pattern := range overflowPatterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	// Bare 400/413 only counts with a size-related keyword — a 400 from a
	// schema or auth problem must not trigger destructive recovery.
	if !httpCodePattern.MatchString(message) {
		return false
	}
	for _, keyword := range []string{"token", "context", "exceed", "too long", "too large", "maximum", "length", "large", "big"} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

var httpCodePattern = regexp.MustCompile(`(?:^|\D)(400|413)(?:\D|$)`)
