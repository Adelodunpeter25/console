// The `console computer-use eval` command: run one JavaScript snippet
// against the driver and print what came back.
//
// This is the fastest way to exercise computer use without a chat: probing,
// debugging a task, and verifying a fresh build all go through here. Each
// invocation uses a throwaway runtime, so nothing persists between calls —
// persistence belongs to chat sessions, not to the command line. Pair with
// --out when launching through open, which provides no terminal.
package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func computerUseEvalCmd() *cobra.Command {
	var code, file, out string
	var timeoutMs int
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run JavaScript against the computer and print the result",
		Long: `Run JavaScript against the computer and print the result.

The snippet runs with the preinstalled cua object, exactly like the computer
tool the agent uses. Variables do not persist: every invocation starts fresh.
Read the /computer-use skill (in the plan docs) for the method reference.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cua.EnsureHostEnv()

			if code == "" && file == "" {
				return fmt.Errorf("pass --code with a snippet or --file with a script path")
			}
			if code != "" && file != "" {
				return fmt.Errorf("pass only one of --code and --file")
			}
			if file != "" {
				raw, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				code = string(raw)
			}
			evalOutputPath = out

			lines, err := runEval(code, time.Duration(timeoutMs)*time.Millisecond)
			if err != nil {
				return err
			}
			return writeEvalOutput(lines)
		},
	}
	cmd.Flags().StringVar(&code, "code", "", "JavaScript snippet to run")
	cmd.Flags().StringVar(&file, "file", "", "Path to a JavaScript file to run")
	cmd.Flags().IntVar(&timeoutMs, "timeout-ms", 0, "Execution budget in milliseconds (default 5 minutes)")
	cmd.Flags().StringVar(&out, "out", "", "Write the result to this file instead of stdout (needed when launched via open)")
	return cmd
}

// evalOutputPath carries the --out destination from flag parsing to output.
// A package-level binding keeps the cobra wiring flat.
var evalOutputPath string

// runEval executes code in a throwaway runtime and renders the outcome as
// printable lines: text parts verbatim, images as size notes (the bytes
// belong in transcripts, not terminals).
func runEval(code string, timeout time.Duration) ([]string, error) {
	out, err := cua.NewManager().EvalOnce(context.Background(), code, timeout)
	if err != nil {
		return nil, err
	}
	parts, ok := out.([]map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%v", out)}, nil
	}
	return renderEvalLines(parts), nil
}

// renderEvalLines reduces harness parts to terminal-safe lines.
func renderEvalLines(parts []map[string]any) []string {
	var lines []string
	for _, part := range parts {
		switch part["type"] {
		case "text":
			if text, _ := part["text"].(string); text != "" {
				lines = append(lines, text)
			}
		case "image":
			// Never print base64 to a terminal: report the size the bytes
			// would decode to, the same honesty as the transcript summaries.
			data, _ := part["data"].(string)
			lines = append(lines, fmt.Sprintf("[image %v, ~%d bytes]",
				part["mimeType"], len(data)*3/4))
		}
	}
	return lines
}

// RunEvalForTest exposes runEval so the no-driver failure path is pinned
// without a chat session or a native library.
func RunEvalForTest(code string, timeout time.Duration) ([]string, error) {
	return runEval(code, timeout)
}

// RenderEvalLinesForTest exposes the terminal rendering so the base64
// discipline is pinned: megabytes must never reach stdout.
func RenderEvalLinesForTest(parts []map[string]any) []string {
	return renderEvalLines(parts)
}

// writeEvalOutput prints the lines, or writes them to --out when the process
// has no terminal to print to.
func writeEvalOutput(lines []string) error {
	if evalOutputPath == "" {
		for _, line := range lines {
			fmt.Println(line)
		}
		return nil
	}
	data := ""
	for _, line := range lines {
		data += line + "\n"
	}
	return os.WriteFile(evalOutputPath, []byte(data), 0o644)
}
