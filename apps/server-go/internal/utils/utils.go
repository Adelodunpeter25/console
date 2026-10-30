// Small shared helpers.
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NowMillis() int64 {
	return time.Now().UnixMilli()
}

func RandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// NowISO returns the UTC timestamp format used for records that carry both
// date and time. The fractional second keeps adjacent records sortable.
func NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}
