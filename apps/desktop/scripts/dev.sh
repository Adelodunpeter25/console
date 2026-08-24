#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$ROOT_DIR"

echo "==> Starting Console Native (GPUI) in watch mode..."

if command -v cargo-watch &> /dev/null; then
    cargo watch -x "run -p console-app"
else
    echo "==> cargo-watch not found, running directly with cargo run..."
    cargo run -p console-app "$@"
fi
