// Per-credential refresh serialization. Two concurrent refreshes of the
// same credential would both POST the token endpoint and both write the
// rotated pair back; with one account per file this is already a latent
// race, and multiple accounts per provider widen it. SingleFlight runs fn
// once per key — losers wait and receive the winner's result instead of
// re-running with an already-rotated refresh token.
package shared

import "sync"

var (
	flightMu      sync.Mutex
	flights       = map[string]chan struct{}{}
	flightResults = map[string]any{}
	flightErrs    = map[string]error{}
)

// SingleFlight deduplicates concurrent fn calls with the same key: the
// first caller runs fn, later callers block until it finishes and get its
// result. New callers after a completed run execute fn again — same
// semantics as antigravity's refresh dedupe, which this mirrors.
func SingleFlight[T any](key string, fn func() (T, error)) (T, error) {
	flightMu.Lock()
	if ch, ok := flights[key]; ok {
		flightMu.Unlock()
		<-ch
		flightMu.Lock()
		out, err := flightResults[key], flightErrs[key]
		flightMu.Unlock()
		if typed, ok := out.(T); ok {
			return typed, err
		}
		var zero T
		return zero, err
	}
	ch := make(chan struct{})
	flights[key] = ch
	flightMu.Unlock()

	out, err := fn()

	flightMu.Lock()
	flightResults[key], flightErrs[key] = out, err
	delete(flights, key)
	flightMu.Unlock()
	close(ch)
	return out, err
}
