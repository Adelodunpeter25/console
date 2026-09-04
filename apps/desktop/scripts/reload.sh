#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE_ROOT="$(cd "$DESKTOP_DIR/../.." && pwd)"

APP_PATH="$DESKTOP_DIR/dist/Console Dev.app"
APP_EXEC="$APP_PATH/Contents/MacOS/console"
PID_FILE="$DESKTOP_DIR/dist/.dev_console.pid"

BINARY_SRC="$DESKTOP_DIR/target/debug/console"
if [[ ! -f "$BINARY_SRC" ]]; then
    BINARY_SRC="$WORKSPACE_ROOT/target/debug/console"
fi

if [[ -f "$BINARY_SRC" ]]; then
    cp -f "$BINARY_SRC" "$APP_EXEC"
    codesign --force --deep --sign - "$APP_PATH" >/dev/null 2>&1 || true
fi

# Kill previous instance if running
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE" 2>/dev/null || true)
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
        kill "$OLD_PID" 2>/dev/null || true
    fi
fi
pkill -f "$APP_EXEC" 2>/dev/null || true

# Launch new instance in background and store PID
export CONSOLE_ENV=dev
"$APP_EXEC" >/dev/null 2>&1 &
echo $! > "$PID_FILE"
