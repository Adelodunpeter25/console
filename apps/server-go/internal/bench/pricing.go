package bench

import (
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// Price is USD per million tokens by billing type.
type Price struct {
	Input, CacheWrite, CacheRead, Output float64
}

// prices by model-id prefix (Anthropic list prices, 5-minute cache
// writes). Unknown models cost 0 and are flagged in the report.
var prices = []struct {
	prefix string
	price  Price
}{
	{"claude-haiku-4", Price{Input: 1, CacheWrite: 1.25, CacheRead: 0.10, Output: 5}},
	{"claude-sonnet-4", Price{Input: 3, CacheWrite: 3.75, CacheRead: 0.30, Output: 15}},
	{"claude-opus-4", Price{Input: 5, CacheWrite: 6.25, CacheRead: 0.50, Output: 25}},
}

// PriceFor returns the model's price and whether it is known.
func PriceFor(model string) (Price, bool) {
	for _, p := range prices {
		if strings.HasPrefix(model, p.prefix) {
			return p.price, true
		}
	}
	return Price{}, false
}

// Cost prices a flat usage in USD.
func Cost(p Price, u loop.RunUsage) float64 {
	return (float64(u.Input)*p.Input + float64(u.CacheWrite)*p.CacheWrite +
		float64(u.CacheRead)*p.CacheRead + float64(u.Output+u.Reasoning)*p.Output) / 1e6
}
