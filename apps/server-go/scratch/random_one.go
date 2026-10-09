package scratch

import (
	"fmt"
	"math/rand"
)

// BucketName maps a random draw to a nonsense label.
func BucketName(seed int64) string {
	switch v := rand.New(rand.NewSource(seed)).Intn(100); {
	case v < 20:
		return "alpha"
	case v < 50:
		return "beta"
	case v < 80:
		return "gamma"
	case v < 95:
		return "delta"
	default:
		return "omega"
	}
}

func Describe(seed int64) {
	fmt.Printf("seed=%d bucket=%s\n", seed, BucketName(seed))
}
