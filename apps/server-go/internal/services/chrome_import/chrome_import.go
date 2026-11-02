// Package chromeimport reads Chrome profile data (profile list, top
// history, decrypted cookies) for the built-in browser's import flow.
//
// The desktop client keeps zero of the weight: SQLite reads (Chrome's
// stores are plain sqlite) and cookie decryption (PBKDF2 + AES-CBC/GCM)
// both hand-roll on stdlib plus golang.org/x/crypto, both already in the
// module graph. Decrypted cookie values never touch disk here — the
// service is a pass-through that returns fresh data per request; the
// desktop injects the values straight into its WKWebView cookie store.
package chromeimport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ChromeProfile is one profile folder plus its human-readable name.
type ChromeProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChromeHistoryEntry is one history row, most visited first.
type ChromeHistoryEntry struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	VisitCount int64  `json:"visit_count"`
}

// ChromeCookie is one decrypted cookie, ready for webview injection.
type ChromeCookie struct {
	Domain      string `json:"domain"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty"` // 0 = session cookie
	Secure      bool   `json:"secure"`
	HTTPOnly    bool   `json:"http_only"`
}

// ChromeBaseDir resolves Chrome's data root on this machine, or "" when
// Chrome has no data here.
func ChromeBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	base := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	info, err := os.Stat(base)
	if err != nil || !info.IsDir() {
		return ""
	}
	return base
}

// DetectProfiles enumerates profile folders that hold data, naming each
// from Chrome's own records (Local State first, per-profile Preferences
// second, folder name last).
func DetectProfiles() []ChromeProfile {
	return DetectProfilesAt(ChromeBaseDir())
}

func DetectProfilesAt(base string) []ChromeProfile {
	if base == "" {
		return nil
	}
	infoCache := profileInfoCache(filepath.Join(base, "Local State"))

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var profiles []ChromeProfile
	for _, entry := range entries {
		folder := entry.Name()
		// Chrome names snapshot folders the same way; only the two shapes a
		// real profile uses are candidates.
		if folder != "Default" && !strings.HasPrefix(folder, "Profile ") {
			continue
		}
		dir := filepath.Join(base, folder)
		if !entry.IsDir() || !hasProfileData(dir) {
			continue
		}
		name := detectProfileName(dir, folder, infoCache)
		profiles = append(profiles, ChromeProfile{ID: folder, Name: name})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles
}

func hasProfileData(dir string) bool {
	for _, p := range []string{"History", filepath.Join("Network", "Cookies"), "Cookies"} {
		if info, err := os.Stat(filepath.Join(dir, p)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// profileInfoCache parses Local State's profile.info_cache, keyed by folder.
func profileInfoCache(localStatePath string) map[string]map[string]any {
	out := map[string]map[string]any{}
	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return out
	}
	var doc struct {
		Profile struct {
			InfoCache map[string]map[string]any `json:"info_cache"`
		} `json:"profile"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return out
	}
	return doc.Profile.InfoCache
}

func detectProfileName(dir, folder string, infoCache map[string]map[string]any) string {
	// Local State carries the real display name ("Peter"); the
	// per-profile Preferences file is the fallback.
	if info, ok := infoCache[folder]; ok {
		for _, key := range []string{"gaia_name", "gaia_given_name", "name"} {
			if name, ok := info[key].(string); ok && strings.TrimSpace(name) != "" {
				return strings.TrimSpace(name)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "Preferences"))
	if err == nil {
		var doc struct {
			Profile struct {
				Name string `json:"name"`
			} `json:"profile"`
		}
		if json.Unmarshal(data, &doc) == nil && strings.TrimSpace(doc.Profile.Name) != "" {
			return strings.TrimSpace(doc.Profile.Name)
		}
	}
	return folder
}

// ReadHistory returns the top `limit` history rows, most visited first.
func ReadHistory(profileID string, limit int) ([]ChromeHistoryEntry, error) {
	dir, err := requireProfileDir(profileID)
	if err != nil {
		return nil, err
	}
	return ReadHistoryAt(dir, limit)
}

func ReadHistoryAt(profileDir string, limit int) ([]ChromeHistoryEntry, error) {
	rows, err := copyAndQueryHistory(profileDir, limit)
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	return rows, nil
}

// ReadCookies returns the profile's live cookies, decrypted, with expired
// and CHIPS-partitioned rows skipped.
func ReadCookies(profileID string) ([]ChromeCookie, error) {
	dir, err := requireProfileDir(profileID)
	if err != nil {
		return nil, err
	}
	return ReadCookiesAt(dir)
}

func ReadCookiesAt(profileDir string) ([]ChromeCookie, error) {
	rows, err := copyAndQueryCookies(profileDir)
	if err != nil {
		return nil, fmt.Errorf("read cookies: %w", err)
	}
	return decryptRows(rows)
}

func requireProfileDir(profileID string) (string, error) {
	if profileID == "" {
		return "", fmt.Errorf("profileId is required")
	}
	// The id comes from DetectProfiles and names a folder under the base;
	// anything with separators is either a probe or inconsistent input.
	if strings.ContainsAny(profileID, "/\\") || profileID == "." || profileID == ".." {
		return "", fmt.Errorf("invalid profile %q", profileID)
	}
	base := ChromeBaseDir()
	if base == "" {
		return "", fmt.Errorf("no Chrome data on this machine")
	}
	dir := filepath.Join(base, profileID)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("no Chrome profile %q", profileID)
	}
	return dir, nil
}
