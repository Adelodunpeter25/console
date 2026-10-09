.PHONY: dev-server dev-console dev-mobile dev-desktop build-desktop package-desktop build-server install build-android check proto help

# Where `console upgrade` and this target install the binary (matches
# resolveUpgradeTarget's CONSOLE_INSTALL_DIR fallback in upgrade.go).
INSTALL_DIR ?= $(HOME)/.local/bin

# Default target
.DEFAULT_GOAL := help

## dev-server: Start the Go agent server in dev mode (uses ~/.console-dev storage)
dev-server:
	CONSOLE_ENV=dev go -C apps/server-go run ./cmd/server

## dev-console: Start the console agent as a background daemon (survives closing terminal)
##   Usage: make dev-console            (dev: port 3000, ~/.console-dev storage)
##          make dev-console PORT=3001  (prod: port 3001, ~/.console storage)
## Dev (default port) sets CONSOLE_ENV=dev so daemon paths resolve
## ~/.console-dev, matching the desktop's separate dev bundle identifier.
## Builds the Go binary first.
dev-console: build-server
	CONSOLE_ENV=$(if $(PORT),,dev) ./console start -p $(if $(PORT),$(PORT),3000)

## dev-mobile: Build and run the native Android app in dev mode
##   Installs debug APK and launches the app on Android emulator/device
##   Requires Android SDK and emulator running or device connected via adb
dev-mobile:
	@if [ ! -f apps/android/key.properties ]; then \
		echo "error: apps/android/key.properties is missing." >&2; \
		echo "Fresh worktrees don't include gitignored signing files, and Gradle would silently fall back to a throwaway debug key." >&2; \
		echo "Link it (and apps/android/keystore) from your main checkout, then verify:" >&2; \
		echo "  cd apps/android && ./gradlew :app:signingReport -q  # debug must show 'Config: release'" >&2; \
		exit 1; \
	fi
	cd apps/android && ./gradlew installDebug && adb shell am start -n com.console.mobile.dev/com.console.mobile.MainActivity

## dev-desktop: Build and launch the GPUI desktop app in dev mode (Console Dev.app)
dev-desktop:
	bash apps/desktop/scripts/dev.sh

## package-desktop: Package the GPUI desktop app into a production macOS .app bundle (Console.app)
package-desktop:
	bash apps/desktop/scripts/package.sh

## build-desktop: Build the GPUI desktop app for production
build-desktop:
	cargo build --release --manifest-path apps/desktop/Cargo.toml

## desktop-check: Fast typecheck of the GPUI desktop app (locked deps)
desktop-check:
	cargo check --locked --manifest-path apps/desktop/Cargo.toml

## build-server: Compile the multi-call `console` binary (Go CLI + agent server)
## (`console start` re-executes itself with CONSOLE_SERVE=1 to BE the daemon)
build-server:
	go -C apps/server-go build -ldflags="-s -w" -trimpath -o ../../console ./cmd/console

## install: Build the Go server from source and swap it into $(INSTALL_DIR)
##   (default ~/.local/bin), stopping and restarting the daemon around the
##   swap. Same effect as `console upgrade` but from your local checkout
##   instead of downloading a release — for testing unreleased fixes.
install: build-server
	@mkdir -p "$(INSTALL_DIR)"
	@was_running=0; \
	if [ -x "$(INSTALL_DIR)/console" ] && "$(INSTALL_DIR)/console" status | grep -q "is running"; then \
		was_running=1; \
		echo "Stopping running daemon..."; \
		"$(INSTALL_DIR)/console" stop; \
	fi; \
	mv ./console "$(INSTALL_DIR)/console"; \
	echo "Installed $(INSTALL_DIR)/console"; \
	if [ "$$was_running" = "1" ]; then \
		echo "Restarting daemon on the new binary..."; \
		"$(INSTALL_DIR)/console" start; \
	else \
		echo "Daemon was not running. Run 'console start' to launch it."; \
	fi

## build-android: Build the native Android app for release
build-android:
	cd apps/android && ./gradlew assembleRelease

## check: Vet the Go server
check:
	go -C apps/server-go vet ./...

## proto: Regenerate Go protobuf types from proto/ (buf preferred, protoc fallback)
proto:
	@if command -v buf >/dev/null 2>&1; then \
		buf generate; \
	else \
		echo "buf not found, falling back to protoc..."; \
		GOBIN=$$(go env GOPATH)/bin; \
		mkdir -p apps/server-go/internal/gen; \
		(ls $$GOBIN/protoc-gen-go >/dev/null 2>&1 || go -C apps/server-go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12); \
		export PATH="$$GOBIN:$$PATH"; \
		protoc -I proto --go_out=apps/server-go/internal/gen --go_opt=paths=source_relative $$(find proto/console/v1 -name '*.proto'); \
	fi

## help: Show this help message
help:
	@echo "Available commands:"
	@echo "  make dev-server        - Start the backend agent server"
	@echo "  make dev-console       - Start the console agent as a background daemon (PORT=nnnn to set port)"
	@echo "  make dev-mobile        - Build and run the native Android app in dev mode"
	@echo "  make dev-desktop       - Start the GPUI desktop app in dev mode"
	@echo "  make package-desktop   - Package the GPUI desktop app for production (.app bundle)"
	@echo "  make build-desktop     - Build the GPUI desktop app for production"
	@echo "  make desktop-check     - Fast typecheck of the GPUI desktop app"
	@echo "  make build-server      - Compile the multi-call console binary (CLI + server)"
	@echo "  make install           - Build from source and install/restart the local console daemon (INSTALL_DIR=path)"
	@echo "  make build-android     - Build the native Android app for release"
	@echo "  make check             - Vet the Go server"
