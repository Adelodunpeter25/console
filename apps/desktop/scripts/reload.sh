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
    # CEF bundles: helpers are our own binary (see build.sh --cef). Dev
    # bundles share ONE signed executable via hardlinks instead of five
    # ~300MB copies. Re-signing with --deep would replace each helper file and
    # sever the links, so sign a single seed with the helper identifier, relink
    # every helper to it, and sign only the app container.
    HELPER_EXECS=()
    for HELPER_EXEC in "$APP_PATH"/Contents/Frameworks/*.app/Contents/MacOS/*; do
        [[ -f "$HELPER_EXEC" ]] && HELPER_EXECS+=("$HELPER_EXEC")
    done
    if [[ ${#HELPER_EXECS[@]} -gt 0 ]]; then
        HELPER_PLIST="$(dirname "$(dirname "${HELPER_EXECS[0]}")")/Info.plist"
        HELPER_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$HELPER_PLIST")"
        HELPER_SEED="$APP_PATH/Contents/Frameworks/.helper-seed"
        rm -f "$HELPER_SEED"
        cp "$BINARY_SRC" "$HELPER_SEED"
        codesign --force --sign - --identifier "$HELPER_ID" "$HELPER_SEED" >/dev/null 2>&1 || true
        for HELPER_EXEC in "${HELPER_EXECS[@]}"; do
            rm -f "$HELPER_EXEC"
            ln "$HELPER_SEED" "$HELPER_EXEC" 2>/dev/null || cp "$HELPER_SEED" "$HELPER_EXEC"
        done
        rm -f "$HELPER_SEED"
    fi
    codesign --force --sign - "$APP_PATH" >/dev/null 2>&1 || true
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
