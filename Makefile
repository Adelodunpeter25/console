.PHONY: dev-server dev-console dev-mobile dev-desktop build-desktop package-desktop build-desktop-mac typecheck check generate-icons generate-theme help

# Default target
.DEFAULT_GOAL := help

## dev-server: Start the Hono agent server in dev mode (uses ~/.console-dev storage)
dev-server:
	CONSOLE_ENV=dev bun --watch apps/server/index.ts

## dev-console: Start the console agent as a background daemon (survives closing terminal)
##   Usage: make dev-console            (dev: port 3000, ~/.console-dev storage)
##          make dev-console PORT=3001  (prod: port 3001, ~/.console storage)
## Dev (default port) sets CONSOLE_ENV=dev so apppaths/daemon-manager resolve
## ~/.console-dev, matching the desktop's separate dev bundle identifier.
dev-console:
	CONSOLE_ENV=$(if $(PORT),,dev) bun apps/cli/index.ts start -p $(if $(PORT),$(PORT),3000)

## dev-mobile: Start the Expo React Native app dev server
dev-mobile:
	bun run --cwd apps/mobile android

## dev-desktop: Build and launch the GPUI desktop app in dev mode (Console Dev.app)
dev-desktop:
	bash apps/desktop/scripts/dev.sh

## package-desktop: Package the GPUI desktop app into a production macOS .app bundle (Console.app)
package-desktop:
	bash apps/desktop/scripts/package.sh

## build-desktop: Build the GPUI desktop app for production
build-desktop:
	cargo build --release --manifest-path apps/desktop/Cargo.toml

## build-server: Compile the multi-call `console` binary (CLI + agent server)
## (bun runtime embedded, minified JS, zstd sourcemap for readable stacktraces)
build-server:
	bun build --compile --minify --sourcemap apps/cli/console.ts --outfile console

## build-preview
build-preview:
	cd apps/mobile && eas build --platform android --profile preview 

## typecheck: Run TypeScript check across all monorepo workspaces
typecheck:
	bunx tsc --noEmit

## generate-icons: Regenerate mobile SVG icon registries from console-rs assets
generate-icons:
	bun run --cwd apps/mobile icons:generate

## generate-theme: Regenerate mobile JS theme from global.css tokens
generate-theme:
	bun run --cwd apps/mobile theme:generate

## check: Run vp check for code formatting and linting
check:
	bun run check

## help: Show this help message
help:
	@echo "Available commands:"
	@echo "  make dev-server        - Start the backend agent server"
	@echo "  make dev-console       - Start the console agent as a background daemon (PORT=nnnn to set port)"
	@echo "  make dev-mobile        - Start the Expo mobile app dev server"
	@echo "  make dev-desktop       - Start the GPUI desktop app in dev mode"
	@echo "  make package-desktop   - Package the GPUI desktop app for production (.app bundle)"
	@echo "  make build-desktop     - Build the GPUI desktop app for production"
	@echo "  make build-server      - Compile the multi-call console binary (CLI + server)"
	@echo "  make build-preview - Build the Android apk with eas"
	@echo "  make typecheck         - Run TypeScript typechecking"
	@echo "  make generate-icons    - Regenerate mobile SVG icon registries from console-rs"
	@echo "  make generate-theme    - Regenerate mobile JS theme from global.css tokens"
	@echo "  make check             - Run Vite+ code format and lint checks"
