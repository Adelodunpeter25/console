// Server entry point. Thin wrapper over internal/serve so `go run
// ./cmd/server` stays a server-only dev path; the distributable multi-call
// binary lives in cmd/console.
package main

import (
	"os"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/serve"
)

func main() {
	if err := serve.Run(); err != nil {
		os.Exit(1)
	}
}
