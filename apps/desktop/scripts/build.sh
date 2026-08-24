#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$ROOT_DIR"

echo "==> Building Console Native (GPUI) in release mode..."
cargo build --release -p console-app "$@"

echo "==> Build complete: $ROOT_DIR/target/release/console"
