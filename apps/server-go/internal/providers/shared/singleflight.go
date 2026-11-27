// Per-credential refresh serialization. Two concurrent refreshes of the
// same credential would both POST the token endpoint and both write the
// rotated pair back; with one account per file this is already a latent
// race, and multiple accounts per provider widen it. SingleFlight runs fn
// once per key — losers wait and receive the winner's result instead of
// re-running with an already-rotated refresh token.
package shared

import "golang.org/x/sync/singleflight"

var flightGroup singleflight.Group

// SingleFlight deduplicates concurrent fn calls with the same key: the
// first caller runs fn, later callers block until it finishes and get its
// result. New callers after a completed run execute fn again.
func SingleFlight[T any](key string, fn func() (T, error)) (T, error) {
	v, err, _ := flightGroup.Do(key, func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		return zero, err
	}
	if typed, ok := v.(T); ok {
		return typed, nil
	}
	var zero T
	return zero, nil
}
