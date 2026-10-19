// Package gitconfig teaches the git CLI about the GitHub credential helper
// for this process and, transitively, every child process it spawns.
//
// Configuration rides the GIT_CONFIG_COUNT / GIT_CONFIG_KEY_n /
// GIT_CONFIG_VALUE_n env channel (git >= 2.31) rather than ~/.gitconfig, so
// nothing global is written and a disconnect leaves no trace on disk.
//
// Injecting into the server's own environment — instead of per-call-site env
// assembly — is what makes this complete. Every git path already inherits
// os.Environ().
package gitconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HelperEnvVar marks a process as a git credential-helper invocation.
const HelperEnvVar = "CONSOLE_GIT_CREDENTIAL_HELPER"

// IdentityEnvName and IdentityEnvEmail allow installed servers to configure
// commit authors without writing ~/.gitconfig. When omitted, Configure tries
// the authenticated GitHub CLI account and falls back silently if gh is not
// installed or is not authenticated.
const (
	IdentityEnvName  = "CONSOLE_GIT_USER_NAME"
	IdentityEnvEmail = "CONSOLE_GIT_USER_EMAIL"
)

type identity struct {
	Name  string `json:"name"`
	Login string `json:"login"`
	Email string `json:"email"`
}

// gitConfigEntries returns the config triples to inject, in order. Each
// becomes one GIT_CONFIG_COUNT slot. The helper value needs the running
// binary's path, so this is a function of exe rather than a package var.
func gitConfigEntries(exe string, author identity) []struct{ key, value string } {
	entries := []struct{ key, value string }{
		{"credential.helper", "!" + HelperEnvVar + "=1 " + shellQuote(exe)},
		{"url.https://github.com/.insteadOf", "git@github.com:"},
		{"url.https://github.com/.insteadOf", "ssh://git@github.com/"},
	}
	if author.Name != "" {
		entries = append(entries, struct{ key, value string }{"user.name", author.Name})
	}
	if author.Email != "" {
		entries = append(entries, struct{ key, value string }{"user.email", author.Email})
	}
	return entries
}

// Configure points git at this binary's credential-helper mode, rewrites
// SSH-style GitHub URLs to HTTPS, and supplies a stable commit identity for
// every git child process. Identity is read from CONSOLE_GIT_USER_* first and
// then from the authenticated gh CLI account. No ~/.gitconfig is modified.
func Configure() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve console binary for the git credential helper: %w", err)
	}
	return ConfigureWithExe(exe)
}

// ConfigureWithExe applies the config with an explicit helper binary path.
func ConfigureWithExe(exe string) error {
	if strings.TrimSpace(exe) == "" {
		return fmt.Errorf("resolve console binary for the git credential helper: empty path")
	}
	entries := gitConfigEntries(exe, resolveIdentity())
	if err := os.Setenv("GIT_CONFIG_COUNT", fmt.Sprint(len(entries))); err != nil {
		return err
	}
	for i, entry := range entries {
		if err := os.Setenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", i), entry.key); err != nil {
			return err
		}
		if err := os.Setenv(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i), entry.value); err != nil {
			return err
		}
	}
	return nil
}

func resolveIdentity() identity {
	out := identity{
		Name:  strings.TrimSpace(os.Getenv(IdentityEnvName)),
		Email: strings.TrimSpace(os.Getenv(IdentityEnvEmail)),
	}
	if out.Name != "" && out.Email != "" {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "user")
	data, err := cmd.Output()
	if err == nil {
		var gh identity
		if json.Unmarshal(data, &gh) == nil {
			if out.Name == "" {
				out.Name = strings.TrimSpace(gh.Name)
				if out.Name == "" {
					out.Name = strings.TrimSpace(gh.Login)
				}
			}
			if out.Email == "" {
				out.Email = strings.TrimSpace(gh.Email)
				if out.Email == "" && gh.Login != "" {
					out.Email = gh.Login + "@users.noreply.github.com"
				}
			}
		}
	}
	return out
}

// shellQuote wraps a path in single quotes for /bin/sh, which git uses to run
// a `!` helper.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
