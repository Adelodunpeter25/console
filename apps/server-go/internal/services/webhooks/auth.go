package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

func verifyAuth(secret string, headers map[string]string, queryToken string, body []byte) bool {
	if secret == "" {
		return false
	}
	if queryToken != "" && subtle.ConstantTimeCompare([]byte(queryToken), []byte(secret)) == 1 {
		return true
	}
	for k, v := range headers {
		switch strings.ToLower(k) {
		case "x-webhook-secret":
			if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(v)), []byte(secret)) == 1 {
				return true
			}
		case "authorization":
			if scheme, token, ok := strings.Cut(strings.TrimSpace(v), " "); ok && strings.EqualFold(scheme, "bearer") {
				if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(secret)) == 1 {
					return true
				}
			}
		case "x-hub-signature-256", "x-webhook-signature":
			if verifyHMACSHA256(secret, v, body) {
				return true
			}
		}
	}
	return false
}

func verifyHMACSHA256(secret, header string, body []byte) bool {
	sigHex := strings.TrimSpace(header)
	if strings.HasPrefix(strings.ToLower(sigHex), "sha256=") {
		sigHex = sigHex[len("sha256="):]
	}
	got, err := hex.DecodeString(sigHex)
	if err != nil || len(got) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(got, expected)
}
