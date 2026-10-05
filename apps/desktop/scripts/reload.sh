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
    # CEF bundles: helpers are copies of our own binary (see build.sh --cef).
    # Refresh them when the fresh binary is newer so renderer/GPU helpers
    # never run stale code after a reload.
    for HELPER_EXEC in "$APP_PATH"/Contents/Frameworks/*.app/Contents/MacOS/*; do
        if [[ -f "$HELPER_EXEC" && "$BINARY_SRC" -nt "$HELPER_EXEC" ]]; then
            cp -f "$BINARY_SRC" "$HELPER_EXEC"
        fi
    done
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
# Keep stdout/stderr in a log file (was /dev/null, which swallowed every
# log:: line). Tail it with: tail -f "$DESKTOP_DIR/dist/.dev_console.log"
LOG_FILE="$DESKTOP_DIR/dist/.dev_console.log"
# --use-mock-keychain: Chromium encrypts cookies/passwords with a key from
# the login keychain ("Chromium Safe Storage"). Dev binaries are ad-hoc
# signed and rebuilt constantly (6 distinct helper copies), so the keychain
# never learns a stable identity and prompts on every launch. The mock
# keychain keeps encryption in memory: no prompts, sites start logged-out
# each launch. Production bundles (Developer ID signed) use the real
# keychain and prompt at most once.
"$APP_EXEC" --use-mock-keychain >>"$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
