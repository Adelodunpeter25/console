// GH_TOKEN injection for child processes. Models reach for the `gh` CLI by
// default, and on a headless box the interactive `gh auth login` browser flow
// is unusable — so every agent-spawned command inherits the stored PAT as
// GH_TOKEN, which gh prefers over its keyring/oauth state. No new tool or
// prompt change is needed for `gh pr create` et al to work.
//
// The token is loaded from the credential file at spawn time, never cached,
// so connecting or disconnecting GitHub takes effect on the next command
// without a server restart. Callers must never log the injected entry; pair
// with RedactSecrets on anything echoed back to the model.
package github

import "strings"

// Only GH_TOKEN is set. gh reads GH_TOKEN first, so GITHUB_TOKEN is
// deliberately left alone — it carries an Actions-ephemeral-token convention
// that other tooling may already rely on. GHE (GH_HOST) is out of scope,
// same as the credential helper.
const ghTokenKey = "GH_TOKEN"

// AppendToEnv returns env with GH_TOKEN=<stored PAT> appended, unless no
// credential is stored or the env already sets GH_TOKEN or GITHUB_TOKEN. An
// explicit value (operator export, per-command override) always wins over the
// stored one — exec duplicate keys resolve first-match-wins, so appending a
// second entry could never override anyway, only confuse.
func AppendToEnv(env []string) []string {
	if hasGHVar(env) {
		return env
	}
	cred, err := LoadCredential()
	if err != nil {
		return env
	}
	if strings.TrimSpace(cred.Token) == "" {
		return env
	}
	return append(env, ghTokenKey+"="+strings.TrimSpace(cred.Token))
}

func hasGHVar(env []string) bool {
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if key == "GH_TOKEN" || key == "GITHUB_TOKEN" {
			return true
		}
	}
	return false
}

// RedactSecrets replaces exact occurrences of the stored token with
// [redacted]. Apply to anything echoed back to the model (tool
// stdout/stderr) so `env` or `printenv GH_TOKEN` can never land the secret
// in a transcript. No credential stored, or empty input: returns the input
// unchanged.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	cred, err := LoadCredential()
	if err != nil {
		return s
	}
	token := strings.TrimSpace(cred.Token)
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[redacted]")
}
