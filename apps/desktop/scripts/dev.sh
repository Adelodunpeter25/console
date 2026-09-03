#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

RUN_APP=true
EXTRA_ARGS=()

for arg in "$@"; do
    if [[ "$arg" == "--no-run" ]]; then
        RUN_APP=false
    else
        EXTRA_ARGS+=("$arg")
    fi
done

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

if [[ "$RUN_APP" == true ]]; then
    APP_PATH="$DESKTOP_DIR/dist/Console Dev.app"
    echo "==> Launching Console Dev ($APP_PATH)..."
    exec "$APP_PATH/Contents/MacOS/console"
fi
