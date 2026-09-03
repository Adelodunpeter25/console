#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$SCRIPT_DIR/build.sh" \
    --mode prod \
    --release \
    --bundle-id com.console.mobile.prod \
    --app-name "Console" \
    "$@"
