// Env command - manage provider API keys for the daemon.
// Port of apps/cli/commands/env.ts: one hidden prompt per known service
// key, empty input keeps the existing value, keys land in ~/.console/env
// (mode 0600). Piped stdin works too: `echo "$KEY" | console env`.
package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Adelodunpeter25/console/apps/cli-go/internal/daemon"
)

// serviceKey is a known service and the env var it reads.
type serviceKey struct {
	service string
	envVar  string
}

// serviceKeys mirrors SERVICE_KEYS in env.ts.
var serviceKeys = []serviceKey{{service: "firecrawl", envVar: "FIRECRAWL_API_KEY"}}

func isValidValue(value string) bool {
	return len(value) > 0 && !strings.ContainsAny(value, "\r\n\"'")
}

// readHiddenLine reads one line without echoing it. Piped input returns the
// first line; a TTY masks typed characters with * like the TS version.
func readHiddenLine() (string, error) {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	restore, err := setRawStty()
	if err != nil {
		// Fall back to a plain (echoed) line read.
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	defer restore()

	var out []byte
	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return strings.TrimSpace(string(out)), nil
		}
		switch b {
		case '\n', '\r', 0x04: // Enter / Ctrl-D
			fmt.Println()
			return strings.TrimSpace(string(out)), nil
		case 0x03: // Ctrl-C
			restore()
			os.Exit(130)
		case 0x7f, 0x08: // Backspace/DEL
			if len(out) > 0 {
				out = out[:len(out)-1]
				fmt.Print("\b \b")
			}
		default:
			out = append(out, b)
			fmt.Print("*")
		}
	}
}

// setRawStty puts stdin into raw, no-echo mode and returns a restore func.
func setRawStty() (func(), error) {
	save, err := exec.Command("stty", "-g").Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(save))
	if err := exec.Command("stty", "-icanon", "-echo", "min", "1").Run(); err != nil {
		return nil, err
	}
	return func() {
		_ = exec.Command("stty", state).Run()
	}, nil
}

// EnvCommand prompts for each known service key and saves answers.
func EnvCommand() error {
	existing := daemon.LoadEnvFile()
	changed := map[string]string{}

	for _, sk := range serviceKeys {
		has := ""
		if existing[sk.envVar] != "" {
			has = " (already set — Enter to keep)"
		}
		fmt.Printf("%s%s:\n> ", sk.envVar, has)
		value, err := readHiddenLine()
		if err != nil {
			return err
		}
		if value == "" {
			continue
		}
		if !isValidValue(value) {
			fmt.Printf("Skipping %s: value must be one line with no quotes.\n", sk.service)
			continue
		}
		changed[sk.envVar] = value
	}

	if len(changed) == 0 {
		fmt.Println("Nothing changed.")
		return nil
	}
	if err := daemon.UpsertEnvValues(changed); err != nil {
		return err
	}
	fmt.Printf("Saved to %s. Run 'console restart' to apply.\n", daemon.EnvFilePath())
	return nil
}
