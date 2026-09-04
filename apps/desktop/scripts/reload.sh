#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_PATH="$DESKTOP_DIR/dist/Console Dev.app"
APP_EXEC="$APP_PATH/Contents/MacOS/console"
BINARY_SRC="$DESKTOP_DIR/target/debug/console"

if [[ ! -f "$BINARY_SRC" ]]; then
    BINARY_SRC="$(cd "$DESKTOP_DIR/../.." && pwd)/target/debug/console"
fi

if [[ -f "$BINARY_SRC" ]]; then
    cp -f "$BINARY_SRC" "$APP_EXEC"
    codesign --force --deep --sign - "$APP_PATH" >/dev/null 2>&1 || true
fi

pkill -f "$APP_EXEC" 2>/dev/null || true
export CONSOLE_ENV=dev
"$APP_EXEC" &
