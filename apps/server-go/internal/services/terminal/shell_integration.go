// Shell integration scripts for interactive terminal sessions.
// Injects hooks into zsh and bash so that standard shells emit OSC 0 title
// escape sequences on command prompt and during command execution.
package terminal

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var (
	integrationDirOnce sync.Once
	integrationDir     string
	integrationErr     error
)

func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return append(out, prefix+value)
}

func getEnvVar(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

func getShellIntegrationDir() (string, error) {
	integrationDirOnce.Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil || cacheDir == "" {
			cacheDir = os.TempDir()
		}
		baseDir := filepath.Join(cacheDir, "console", "shell-integration")
		if err := setupIntegrationFiles(baseDir); err != nil {
			integrationErr = err
			return
		}
		integrationDir = baseDir
	})
	return integrationDir, integrationErr
}

func setupIntegrationFiles(baseDir string) error {
	zshDir := filepath.Join(baseDir, "zsh")
	if err := os.MkdirAll(zshDir, 0755); err != nil {
		return err
	}
	bashDir := filepath.Join(baseDir, "bash")
	if err := os.MkdirAll(bashDir, 0755); err != nil {
		return err
	}

	zshenvContent := `# Console zsh shell integration
if [[ -n "$CONSOLE_ORIG_ZDOTDIR" ]]; then
  _USER_ZDOTDIR="$CONSOLE_ORIG_ZDOTDIR"
else
  _USER_ZDOTDIR="$HOME"
fi

if [[ -f "$_USER_ZDOTDIR/.zshenv" ]]; then
  source "$_USER_ZDOTDIR/.zshenv"
fi
`
	if err := os.WriteFile(filepath.Join(zshDir, ".zshenv"), []byte(zshenvContent), 0644); err != nil {
		return err
	}

	zprofileContent := `# Console zsh shell integration
if [[ -n "$CONSOLE_ORIG_ZDOTDIR" ]]; then
  _USER_ZDOTDIR="$CONSOLE_ORIG_ZDOTDIR"
else
  _USER_ZDOTDIR="$HOME"
fi

if [[ -f "$_USER_ZDOTDIR/.zprofile" ]]; then
  source "$_USER_ZDOTDIR/.zprofile"
fi
`
	if err := os.WriteFile(filepath.Join(zshDir, ".zprofile"), []byte(zprofileContent), 0644); err != nil {
		return err
	}

	zshrcContent := `# Console zsh shell integration
if [[ -n "$CONSOLE_ORIG_ZDOTDIR" ]]; then
  _USER_ZDOTDIR="$CONSOLE_ORIG_ZDOTDIR"
else
  _USER_ZDOTDIR="$HOME"
fi

# Restore original ZDOTDIR so user environment and child processes behave normally
if [[ -n "$CONSOLE_ORIG_ZDOTDIR" ]]; then
  ZDOTDIR="$CONSOLE_ORIG_ZDOTDIR"
else
  unset ZDOTDIR
fi

if [[ -f "$_USER_ZDOTDIR/.zshrc" ]]; then
  source "$_USER_ZDOTDIR/.zshrc"
fi

autoload -Uz add-zsh-hook 2>/dev/null

__console_title_precmd() {
  local dir_name="${PWD/#$HOME/~}"
  printf '\033]0;%s\007' "$dir_name"
}

__console_title_preexec() {
  local cmd="$1"
  cmd="${cmd#"${cmd%%[![:space:]]*}"}"
  cmd="${cmd%%$'\n'*}"
  if [[ -n "$cmd" ]]; then
    printf '\033]0;%s\007' "$cmd"
  fi
}

add-zsh-hook precmd __console_title_precmd 2>/dev/null
add-zsh-hook preexec __console_title_preexec 2>/dev/null
`
	if err := os.WriteFile(filepath.Join(zshDir, ".zshrc"), []byte(zshrcContent), 0644); err != nil {
		return err
	}

	zloginContent := `# Console zsh shell integration
_USER_ZDOTDIR="${CONSOLE_ORIG_ZDOTDIR:-$HOME}"
if [[ -f "$_USER_ZDOTDIR/.zlogin" ]]; then
  source "$_USER_ZDOTDIR/.zlogin"
fi
`
	if err := os.WriteFile(filepath.Join(zshDir, ".zlogin"), []byte(zloginContent), 0644); err != nil {
		return err
	}

	bashrcContent := `# Console bash shell integration
if [[ -f "$HOME/.bashrc" ]]; then
  source "$HOME/.bashrc"
fi

__console_ready=0

__console_prompt_begin() {
  __console_ready=0
  local dir_name="${PWD/#$HOME/~}"
  printf '\033]0;%s\007' "$dir_name"
}

# Armed only after the whole prompt command ran, so the DEBUG trap ignores
# rc-file and PROMPT_COMMAND commands and fires once per typed command line.
__console_prompt_end() {
  __console_ready=1
}

__console_title_preexec() {
  if [[ "$__console_ready" == 1 ]]; then
    __console_ready=0
    local cmd="${BASH_COMMAND%%$'\n'*}"
    printf '\033]0;%s\007' "$cmd"
  fi
}

trap '__console_title_preexec' DEBUG
PROMPT_COMMAND="__console_prompt_begin${PROMPT_COMMAND:+; $PROMPT_COMMAND}; __console_prompt_end"
`
	return os.WriteFile(filepath.Join(bashDir, "bashrc"), []byte(bashrcContent), 0644)
}

func configureShell(shell string, env []string) ([]string, []string) {
	updatedEnv := setEnvVar(env, "TERM_PROGRAM", "console")
	updatedEnv = setEnvVar(updatedEnv, "TERM_PROGRAM_VERSION", "1.0.0")
	updatedEnv = setEnvVar(updatedEnv, "COLORTERM", "truecolor")

	intDir, err := getShellIntegrationDir()
	if err != nil {
		return nil, updatedEnv
	}

	base := path.Base(shell)
	switch base {
	case "zsh":
		origZdotdir := getEnvVar(env, "ZDOTDIR")
		if origZdotdir != "" {
			updatedEnv = setEnvVar(updatedEnv, "CONSOLE_ORIG_ZDOTDIR", origZdotdir)
		}
		zshDir := filepath.Join(intDir, "zsh")
		updatedEnv = setEnvVar(updatedEnv, "ZDOTDIR", zshDir)
		return []string{"-l"}, updatedEnv

	case "bash":
		bashrcPath := filepath.Join(intDir, "bash", "bashrc")
		return []string{"--rcfile", bashrcPath, "-i"}, updatedEnv

	case "fish":
		return []string{"-l"}, updatedEnv

	default:
		return nil, updatedEnv
	}
}
