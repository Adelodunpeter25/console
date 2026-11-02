package chromeimport

import (
	"os"
	"testing"
)

// TestRealChromeImport exercises the actual local Chrome profile databases
// and the macOS Keychain item, rather than encrypting a fixture with the
// same code the test then decrypts. It is intentionally opt-in because it
// reads personal browser data and may trigger macOS Keychain consent:
//
//	CONSOLE_TEST_REAL_CHROME=1 go test ./internal/services/chrome_import -run '^TestRealChromeImport$' -v
//
// The test reports counts only; it never logs URLs, cookie names/values, or
// Keychain material.
func TestRealChromeImport(t *testing.T) {
	if os.Getenv("CONSOLE_TEST_REAL_CHROME") != "1" {
		t.Skip("set CONSOLE_TEST_REAL_CHROME=1 to read local Chrome data and Keychain")
	}

	profiles := DetectProfiles()
	if len(profiles) == 0 {
		t.Fatal("no Chrome profiles with local data found")
	}

	var totalHistory, totalCookies int
	var profileWithCookies string
	for _, profile := range profiles {
		history, err := ReadHistory(profile.ID, 1000)
		if err != nil {
			t.Errorf("profile %s: read history: %v", profile.ID, err)
			continue
		}
		cookies, err := ReadCookies(profile.ID)
		if err != nil {
			t.Errorf("profile %s: read cookies: %v", profile.ID, err)
			continue
		}
		t.Logf("profile %s: history entries=%d, decrypted cookies=%d", profile.ID, len(history), len(cookies))
		totalHistory += len(history)
		totalCookies += len(cookies)
		if len(cookies) > 0 {
			profileWithCookies = profile.ID
		}
	}

	if totalHistory == 0 {
		t.Error("no history entries were read from any Chrome profile")
	}
	if totalCookies == 0 {
		t.Fatal("no cookies decrypted from any real Chrome profile; check Keychain access and cookie decryption")
	}
	t.Logf("real Chrome import verified: profiles=%d, history entries=%d, decrypted cookies=%d, first profile with cookies=%s",
		len(profiles), totalHistory, totalCookies, profileWithCookies)
}
