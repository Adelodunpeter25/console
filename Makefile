.PHONY: dev-server dev-console dev-mobile dev-desktop build-desktop build-desktop-mac typecheck check generate-icons generate-theme help

# Default target
.DEFAULT_GOAL := help

## dev-server: Start the Hono agent server in dev mode (uses ~/.console-dev storage)
dev-server:
	CONSOLE_ENV=dev npm run dev:server

## dev-console: Start the console agent as a background daemon (survives closing terminal)
##   Usage: make dev-console            (dev: port 3000, ~/.console-dev storage)
##          make dev-console PORT=3001  (prod: port 3001, ~/.console storage)
## Dev (default port) sets CONSOLE_ENV=dev so apppaths/daemon-manager resolve
## ~/.console-dev, matching the desktop's separate dev bundle identifier. Passing
## PORT leaves CONSOLE_ENV unset so prod uses ~/.console. Invoked via tsx because
## chained `npm run` drops the -p/--port flag.
dev-console:
	CONSOLE_ENV=$(if $(PORT),,dev) ./node_modules/.bin/tsx apps/cli/index.ts start -p $(if $(PORT),$(PORT),3000)

## dev-mobile: Start the Expo React Native app dev server
dev-mobile:
	npm run dev:mobile

## dev-desktop: Start the GPUI desktop app in dev mode (requires server running)
dev-desktop:
	cargo run --manifest-path apps/desktop/Cargo.toml

## build-desktop: Build the GPUI desktop app for production
build-desktop:
	cargo build --release --manifest-path apps/desktop/Cargo.toml

## build-desktop-mac: Build the GPUI desktop app for macOS (release)
build-desktop-mac:
	cargo build --release --manifest-path apps/desktop/Cargo.toml

## typecheck: Run TypeScript check across all monorepo workspaces
typecheck:
	npm run typecheck

## generate-icons: Regenerate mobile SVG icon registries from console-rs assets
generate-icons:
	npm run icons:generate

## generate-theme: Regenerate mobile JS theme from global.css tokens
generate-theme:
	npm run theme:generate

## check: Run vp check for code formatting and linting
check:
	npm run check

## help: Show this help message
help:
	@echo "Available commands:"
	@echo "  make dev-server        - Start the backend agent server"
	@echo "  make dev-console       - Start the console agent as a background daemon (PORT=nnnn to set port)"
	@echo "  make dev-mobile        - Start the Expo mobile app dev server"
	@echo "  make dev-desktop       - Start the GPUI desktop app in dev mode"
	@echo "  make build-desktop     - Build the GPUI desktop app for production"
	@echo "  make build-desktop-mac - Build the GPUI desktop app for macOS (release)"
	@echo "  make typecheck         - Run TypeScript typechecking"
	@echo "  make generate-icons    - Regenerate mobile SVG icon registries from console-rs"
	@echo "  make generate-theme    - Regenerate mobile JS theme from global.css tokens"
	@echo "  make check             - Run Vite+ code format and lint checks"
