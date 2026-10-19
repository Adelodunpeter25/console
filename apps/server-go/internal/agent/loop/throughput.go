// Output throughput for one assistant turn: tokens the model produced over
// the time it was actually streaming. Measured server-side because this is
// the only layer that knows when the provider stream opened and closed, and
// because the value is persisted with the turn (so it survives reload).
package loop

import "time"

// minThroughputStream keeps trivially short turns from reporting absurd
// rates: three tokens in 40ms would otherwise read as "75 tok/s".
const minThroughputStream = 250 * time.Millisecond

// tokensPerSecond returns output tokens (reasoning included) per second of
// provider-active stream time, or nil when the result would be meaningless:
// the provider reported no usage, the turn produced no tokens, or the stream
// was too short to average over.
//
// Providers differ on where thinking tokens are counted: Claude folds them
// into Output and leaves ReasoningTokens nil, while Codex, OpenCode and
// Antigravity report them separately. Adding the optional field on top of
// Output covers both without double counting.
func tokensPerSecond(usage *TurnUsage, streamed time.Duration) *float64 {
	if usage == nil || streamed < minThroughputStream {
		return nil
	}
	tokens := usage.Output
	if usage.ReasoningTokens != nil {
		tokens += *usage.ReasoningTokens
	}
	if tokens <= 0 {
		return nil
	}
	rate := float64(tokens) / streamed.Seconds()
	return &rate
}
