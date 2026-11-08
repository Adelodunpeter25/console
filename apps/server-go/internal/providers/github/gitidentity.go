// Git author identity for agent-spawned commands. A headless box has no
// ~/.gitconfig, so the model's first `git commit` fails with "Author identity
// unknown". The connected GitHub account supplies one.
//
// It is delivered as a git config file that sits *under* the machine's own
// global config (the file includes it), pointed at by GIT_CONFIG_GLOBAL. That
// keeps git's normal precedence: a repo's local config and the user's own
// global identity both win, and nothing on the machine is rewritten.
// GIT_AUTHOR_* env vars would be simpler but override even a repo's local
// identity.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const gitConfigGlobalKey = "GIT_CONFIG_GLOBAL"

// FetchIdentity reads the account's numeric id, display name and public email
// from GET /user. Best effort: a failure only means no git identity is set up.
func FetchIdentity(ctx context.Context, client *http.Client, token string) (id int64, name, email string, err error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/user", nil)
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "console-agent")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("GitHub returned %d", resp.StatusCode)
	}
	var parsed struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, "", "", err
	}
	return parsed.ID, strings.TrimSpace(parsed.Name), strings.TrimSpace(parsed.Email), nil
}

// gitIdentity picks the commit identity: the account's public email when
// GitHub gave one, otherwise its private no-reply address, which needs the
// numeric id and always attributes commits to the account.
func (c Credential) gitIdentity() (name, email string, ok bool) {
	name = strings.TrimSpace(c.Name)
	if name == "" {
		name = c.Username
	}
	switch {
	case strings.TrimSpace(c.Email) != "":
		email = strings.TrimSpace(c.Email)
	case c.ID > 0 && c.Username != "":
		email = strconv.FormatInt(c.ID, 10) + "+" + c.Username + "@users.noreply.github.com"
	}
	return name, email, name != "" && email != ""
}

// gitConfigPath is the generated identity file, beside the credential file.
func gitConfigPath() string {
	return filepath.Join(filepath.Dir(CredentialPath()), "git-identity.gitconfig")
}

// writeGitConfig writes the identity file, or removes it when the credential
// carries no usable identity. [user] comes first and the machine's own global
// configs are included after it, so theirs win where they set a value.
func writeGitConfig(cred Credential) error {
	name, email, ok := cred.gitIdentity()
	if !ok {
		return removeGitConfig()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[user]\n\tname = %s\n\temail = %s\n", gitConfigValue(name), email)
	if home, err := os.UserHomeDir(); err == nil {
		for _, p := range []string{
			filepath.Join(home, ".gitconfig"),
			filepath.Join(home, ".config", "git", "config"),
		} {
			if _, err := os.Stat(p); err == nil {
				fmt.Fprintf(&b, "[include]\n\tpath = %s\n", p)
			}
		}
	}
	return os.WriteFile(gitConfigPath(), []byte(b.String()), 0o600)
}

func removeGitConfig() error {
	if err := os.Remove(gitConfigPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// gitConfigValue quotes a value for a git config file.
func gitConfigValue(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}
