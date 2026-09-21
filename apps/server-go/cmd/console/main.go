// Multi-call binary entry point.
//
// One compiled `console` executable serves two roles:
//   - default:        the management CLI (start/stop/status/logs/restart)
//   - CONSOLE_SERVE=1: the agent server itself
//
// `console start` re-executes this same binary detached with CONSOLE_SERVE=1
// so a single installed file can both manage and BE the daemon.
package main

import (
	"os"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/serve"
)

func main() {
	if os.Getenv("CONSOLE_SERVE") == "1" {
		if err := serve.Run(); err != nil {
			os.Exit(1)
		}
		return
	}
	cli.Execute()
}
