// Cloudflare Workers AI speech-to-text: the server-side credential store
// behind /api/stt.
//
// The credential belongs to the machine, not to a paired client: a token
// pasted from desktop or Android lands in the same 0600 file, and only this
// server ever sends it upstream. One active Cloudflare account is enough per
// machine; the status/config wire types are objects, so named profiles can be
// added later without breaking clients.
package stt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	// Model is Cloudflare's turbo Whisper variant: near full-whisper quality
	// at noticeably lower latency.
	Model = "@cf/openai/whisper-large-v3-turbo"
	// MaxAudioBytes matches Cloudflare's audio input cap for CPU models. The
	// HTTP route allows a little more than this so multipart overhead does not
	// reject a payload that is itself within the limit.
	MaxAudioBytes = 4 << 20

	SourceStored = "stored"
	SourceEnv    = "env"
)

var (
	// ErrNotConfigured is returned before any network call so an unconfigured
	// server reports the fixable problem instead of an opaque upstream 401.
	ErrNotConfigured  = errors.New("speech-to-text is not configured; add a Cloudflare account ID and API token")
	ErrEmptyAccountID = errors.New("a Cloudflare account ID is required")
	ErrEmptyAPIToken  = errors.New("a Cloudflare API token is required")
	ErrEmptyAudio     = errors.New("audio is required")
	ErrAudioTooLarge  = fmt.Errorf("audio must be %d MB or smaller", MaxAudioBytes>>20)
)

// Stored is the on-disk credential. The token never leaves this file except on
// the outbound Cloudflare request; status responses carry only accountID.
type Stored struct {
	AccountID string    `json:"accountId"`
	APIToken  string    `json:"apiToken"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Credentials is the resolved pair used for one upstream request.
type Credentials struct {
	AccountID string
	APIToken  string
	Source    string
}

// Status is the token-free view returned by GET /api/stt/status.
type Status struct {
	Configured    bool
	Source        string
	AccountID     string
	Model         string
	UpdatedAt     time.Time
	MaxAudioBytes int
}

// Path resolves the store file. CONSOLE_STT_CONFIG_PATH wins (tests, unusual
// deployments), otherwise <console storage>/stt.json.
func Path() string {
	if override := os.Getenv("CONSOLE_STT_CONFIG_PATH"); override != "" {
		return override
	}
	return filepath.Join(utils.ConsoleStorageDir(), "stt.json")
}

// Load reads the stored credential. A missing file is an empty config, not an
// error, so "never configured" is the normal first-run state.
func Load() (Stored, error) {
	raw, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return Stored{}, nil
		}
		return Stored{}, err
	}
	var stored Stored
	if err := json.Unmarshal(raw, &stored); err != nil {
		// A corrupt file must not be silently overwritten, or one bad write
		// would destroy the user's credential.
		return Stored{}, fmt.Errorf("parse speech-to-text config: %w", err)
	}
	return stored, nil
}

// save writes the store (dir 0700, file 0600). The directory mode is
// re-asserted because ~/.console is created by other subsystems at the process
// umask, which is typically 0755.
func save(stored Stored) error {
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(data, '\n'), 0o600)
}

// Set stores the machine's Cloudflare account pair, replacing any previous
// one. Both halves are required: a stored account ID without a token would
// read as configured while every upstream call failed.
func Set(accountID, apiToken string) error {
	accountID = strings.TrimSpace(accountID)
	apiToken = strings.TrimSpace(apiToken)
	if accountID == "" {
		return ErrEmptyAccountID
	}
	if apiToken == "" {
		return ErrEmptyAPIToken
	}
	return save(Stored{AccountID: accountID, APIToken: apiToken, UpdatedAt: time.Now().UTC()})
}

// Clear removes the stored credential. Removing one that is not set succeeds,
// so a disconnect button is idempotent. A complete env pair still wins after
// this, and status keeps reporting it.
func Clear() error {
	if err := os.Remove(Path()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// envCredentials returns the env pair only when both halves are present and
// non-blank. A partial pair falls through to the stored config rather than
// shadowing it with an unusable credential.
func envCredentials() (Credentials, bool) {
	accountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	apiToken := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if accountID == "" || apiToken == "" {
		return Credentials{}, false
	}
	return Credentials{AccountID: accountID, APIToken: apiToken, Source: SourceEnv}, true
}

// Resolve returns the credential the next upstream call must use. Env wins
// over the stored pair, matching the provider-key convention.
func Resolve() (Credentials, error) {
	if creds, ok := envCredentials(); ok {
		return creds, nil
	}
	stored, err := Load()
	if err != nil {
		return Credentials{}, err
	}
	if stored.AccountID == "" || stored.APIToken == "" {
		return Credentials{}, ErrNotConfigured
	}
	return Credentials{
		AccountID: stored.AccountID,
		APIToken:  stored.APIToken,
		Source:    SourceStored,
	}, nil
}

// CurrentStatus reports whether transcription is configured without exposing
// the token. Static fields (model, upload cap) are always present so clients
// do not hardcode them.
func CurrentStatus() (Status, error) {
	status := Status{Model: Model, MaxAudioBytes: MaxAudioBytes}
	if creds, ok := envCredentials(); ok {
		status.Configured = true
		status.Source = SourceEnv
		status.AccountID = creds.AccountID
		return status, nil
	}
	stored, err := Load()
	if err != nil {
		return Status{}, err
	}
	if stored.AccountID != "" && stored.APIToken != "" {
		status.Configured = true
		status.Source = SourceStored
		status.AccountID = stored.AccountID
		status.UpdatedAt = stored.UpdatedAt
	}
	return status, nil
}
