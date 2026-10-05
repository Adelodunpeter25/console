#!/usr/bin/env bash
set -euo pipefail

# CEF binary cache: with the `cef-browser` cargo feature, the `cef` crate
# downloads the Chromium distribution once here instead of once per target
# directory and build profile. Seeded from a previous build's
# `target/.../out/cef_macos_*` (must contain `archive.json`).
export CEF_PATH="${CEF_PATH:-$HOME/.local/share/cef}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

RUN_APP=true
WATCH_MODE=true
WITH_CEF=false
EXTRA_ARGS=()

for arg in "$@"; do
    if [[ "$arg" == "--no-run" ]]; then
        RUN_APP=false
    elif [[ "$arg" == "--no-watch" ]]; then
        WATCH_MODE=false
    elif [[ "$arg" == "--cef" ]]; then
        # Chromium browser backend: forwarded to build.sh (bundle assembly)
        # and kept on watch-mode rebuilds below so the feature never drops.
        WITH_CEF=true
    else
        EXTRA_ARGS+=("$arg")
    fi
done

build_dev_bundle() {
    if [[ "$WITH_CEF" == true ]]; then
        EXTRA_ARGS+=("--cef")
    fi
    if [[ ${#EXTRA_ARGS[@]} -gt 0 ]]; then
        "$SCRIPT_DIR/build.sh" \
            --mode dev \
            --bundle-id com.console.desktop.dev \
            --app-name "Console Dev" \
            "${EXTRA_ARGS[@]}"
    else
        "$SCRIPT_DIR/build.sh" \
            --mode dev \
            --bundle-id com.console.desktop.dev \
            --app-name "Console Dev"
    fi
}

APP_PATH="$DESKTOP_DIR/dist/Console Dev.app"
APP_EXEC="$APP_PATH/Contents/MacOS/console"

if [[ "$RUN_APP" == false ]]; then
    build_dev_bundle
    exit 0
fi

# Initial build
build_dev_bundle

if [[ "$WATCH_MODE" == true ]] && command -v cargo-watch >/dev/null 2>&1; then
    echo "==> Starting Console Dev in watch mode (auto-reload on save)..."
    export CONSOLE_ENV=dev
    # Default log filter: our crates at debug (terminal key tracing etc.),
    # everything else at warn. Override per-run, e.g. RUST_LOG=debug ./scripts/dev.sh
    # Logs land in dist/.dev_console.log — tail it in another terminal:
    #   tail -f "$DESKTOP_DIR/dist/.dev_console.log"
    export RUST_LOG="${RUST_LOG:-warn,console_ui=debug,console_core=debug,console_app=debug}"
    cd "$DESKTOP_DIR"

    # Trap to kill app when user hits Ctrl+C in terminal
    cleanup() {
        echo ""
        echo "==> Stopping Console Dev..."
        if [[ -f "$DESKTOP_DIR/dist/.dev_console.pid" ]]; then
            PID=$(cat "$DESKTOP_DIR/dist/.dev_console.pid" 2>/dev/null || true)
            if [[ -n "$PID" ]]; then
                kill "$PID" 2>/dev/null || true
            fi
            rm -f "$DESKTOP_DIR/dist/.dev_console.pid"
        fi
        pkill -f "$APP_EXEC" 2>/dev/null || true
        exit 0
    }
    trap cleanup SIGINT SIGTERM

    # Launch initial app
    "$SCRIPT_DIR/reload.sh"

    # cargo watch with --postpone so it waits for file changes instead of immediately rebuilding
    WATCH_BUILD="build -p console-app"
    if [[ "$WITH_CEF" == true ]]; then
        WATCH_BUILD="$WATCH_BUILD --features cef-browser"
    fi
    cargo watch \
        --postpone \
        -w "$DESKTOP_DIR/src" \
        -w "$DESKTOP_DIR/crates" \
        -x "$WATCH_BUILD" \
        -s "$SCRIPT_DIR/reload.sh"
else
    echo "==> Launching Console Dev ($APP_PATH)..."
    export CONSOLE_ENV=dev
    export RUST_LOG="${RUST_LOG:-warn,console_ui=debug,console_core=debug,console_app=debug}"
    exec "$APP_EXEC"
fi
