// The pairing code: the short, human-typable secret a user copies off the
// server and pastes into a client.
//
// It replaces the 43-character random blob the first implementation used.
// That was perfectly secure and completely impractical — nobody types
// "cp1_kJ8xN2mQ..." by hand, and a code nobody can read is a code nobody
// will use.
//
// The trade is entropy for length, so the format is chosen carefully:
//
//   - 12 characters from a 31-symbol alphabet (a-z without i, l and o, plus
//     2-9 without 0 and 1 — the characters people confuse with each other
//     are the ones left out), which is 31^12 ≈ 7.9e17 combinations.
//   - Grouped 4-4-4 with dashes, so it can be read out loud or copied
//     without transcription errors.
//   - Case-insensitive on the way in, because a phone keyboard that
//     auto-capitalises the first letter would otherwise break the code.
//
// The code is short-lived and single-use, and a lookup is a database read
// on a hashed value, so guessing one is not a practical attack: 7.9e17
// attempts is not something that fits in the code's lifetime.
package auth

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	// pairingCodeLength is how many characters the code carries.
	pairingCodeLength = 12
	// pairingCodeGroup is how many characters sit between dashes.
	pairingCodeGroup = 4
	// pairingCodeAlphabet is the 31 symbols a code is built from.
	//
	// Lowercase only: upper and lower case are too easy to confuse when a
	// code is read aloud or typed, and the input is normalised to lower
	// case anyway, so there is nothing to gain from mixing them.
	pairingCodeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
)

// randomPairingCode mints a new pairing code, formatted for reading.
func randomPairingCode() (string, error) {
	raw := make([]byte, pairingCodeLength)
	for i := range raw {
		// Rejection sampling: a plain modulo would bias the low symbols
		// (256 is not a multiple of 31), which quietly costs entropy.
		buf := make([]byte, 1)
		for {
			if _, err := rand.Read(buf); err != nil {
				return "", err
			}
			// 256 = 8*31 + 8, so the top 8 values are rejected.
			if int(buf[0]) < (256/len(pairingCodeAlphabet))*len(pairingCodeAlphabet) {
				raw[i] = pairingCodeAlphabet[int(buf[0])%len(pairingCodeAlphabet)]
				break
			}
		}
	}
	return formatPairingCode(string(raw)), nil
}

// formatPairingCode groups a bare code into dash-separated blocks.
func formatPairingCode(raw string) string {
	var out strings.Builder
	for i, r := range raw {
		if i > 0 && i%pairingCodeGroup == 0 {
			out.WriteByte('-')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// normalizePairingCode reduces a typed code to the form that is hashed and
// looked up: lower case, letters and digits only.
//
// The dashes, the spacing and the capitalisation are all presentation. A
// user pasting "K7MT 2Q9X-4BWN" and one pasting "k7mt2q9x4bwn" mean the
// same thing, and both must work.
func normalizePairingCode(code string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(code)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// validatePairingCode reports whether a code has the right shape.
//
// It is a guard against a fat-fingered paste reaching the database, not a
// security check: the real validation is the hash lookup.
func validatePairingCode(code string) error {
	normalized := normalizePairingCode(code)
	if len(normalized) != pairingCodeLength {
		return fmt.Errorf("a pairing code is %d characters", pairingCodeLength)
	}
	for _, r := range normalized {
		if !strings.ContainsRune(pairingCodeAlphabet, r) {
			return fmt.Errorf("that pairing code has a character it cannot contain")
		}
	}
	return nil
}
