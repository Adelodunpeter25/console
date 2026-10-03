// Multi-call binary entry point.
//
// One compiled `console` executable serves two roles:
//   - default:        the management CLI (start/stop/status/logs/logs) and,
//     via a hidden subcommand, the git credential helper
//   - CONSOLE_SERVE=1: the agent server itself
//
// `console start` re-executes this same binary detached with CONSOLE_SERVE=1
// so a single installed file can both manage and BE the daemon.
package main

import (
	"os"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/gitconfig"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/serve"
)

func main() {
	// Credential-helper mode is checked FIRST, before the serve branch, and
	// this ordering is load-bearing.
	//
	// git invokes the helper as a child of whatever process ran git. When
	// that is the daemon (agent bash tool, runGit, or a PTY shell) the child
	// inherits CONSOLE_SERVE=1, so if the serve check came first the helper
	// would boot a second copy of the whole server instead of printing
	// credentials. The env var is set inline by the credential.helper config
	// value that internal/gitconfig injects, so it is present exactly when
	// git is the caller and absent for an interactive `console`.
	//
	// Routing through a hidden cobra subcommand instead would NOT be safe
	// here: cobra is only reached after the serve check below.
	if os.Getenv(gitconfig.HelperEnvVar) == "1" {
		op := ""
		if len(os.Args) > 1 {
			op = os.Args[1]
		}
		os.Exit(github.RunCredentialHelper(op, os.Stdin, os.Stdout))
	}

	if os.Getenv("CONSOLE_SERVE") == "1" {
		if err := serve.Run(); err != nil {
			os.Exit(1)
		}
		return
	}
	cli.Execute()
}
