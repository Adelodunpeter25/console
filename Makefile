.PHONY: dev-server dev-console dev-mobile dev-desktop build-desktop typecheck check help

# Default target
.DEFAULT_GOAL := help

## dev-server: Start the Hono agent server in dev mode
dev-server:
	npm run dev:server

## dev-console: Start the console agent as a background daemon (survives closing terminal)
##   Usage: make dev-console            (port defaults to 3000)
##          make dev-console PORT=3001
## Invokes the CLI directly via tsx because chained `npm run` drops the -p/--port flag.
dev-console:
	./node_modules/.bin/tsx apps/cli/index.ts start -p $(if $(PORT),$(PORT),3000)

## dev-mobile: Start the Expo React Native app dev server
dev-mobile:
	npm run dev:mobile

## dev-desktop: Start the Tauri desktop app in dev mode (separate dev bundle identifier, requires server running)
dev-desktop:
	npm run dev:desktop

## build-desktop: Build the Tauri desktop app for production
build-desktop:
	npm run build:desktop

## typecheck: Run TypeScript check across all monorepo workspaces
typecheck:
	npm run typecheck

## check: Run vp check for code formatting and linting
check:
	npm run check

## help: Show this help message
help:
	@echo "Available commands:"
	@echo "  make dev-server  - Start the backend agent server"
	@echo "  make dev-console - Start the console agent as a background daemon (PORT=nnnn to set port)"
	@echo "  make dev-mobile  - Start the Expo mobile app dev server"
	@echo "  make dev-desktop  - Start the Tauri desktop app dev server"
	@echo "  make build-desktop  - Build the Tauri desktop app for production"
	@echo "  make typecheck   - Run TypeScript typechecking"
	@echo "  make check       - Run Vite+ code format and lint checks"
