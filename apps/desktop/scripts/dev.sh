#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

RUN_APP=true
WATCH_MODE=true
EXTRA_ARGS=()

for arg in "$@"; do
    if [[ "$arg" == "--no-run" ]]; then
        RUN_APP=false
    elif [[ "$arg" == "--no-watch" ]]; then
        WATCH_MODE=false
    else
        EXTRA_ARGS+=("$arg")
    fi
done

build_dev_bundle() {
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
    cargo watch \
        --postpone \
        -w "$DESKTOP_DIR/src" \
        -w "$DESKTOP_DIR/crates" \
        -x "build -p console-app" \
        -s "$SCRIPT_DIR/reload.sh"
else
    echo "==> Launching Console Dev ($APP_PATH)..."
    export CONSOLE_ENV=dev
    exec "$APP_EXEC"
fi
