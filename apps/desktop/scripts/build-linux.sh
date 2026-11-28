#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE_ROOT="$(cd "$DESKTOP_DIR/../.." && pwd)"

IS_RELEASE=true
OUT_DIR="$DESKTOP_DIR/dist"
TARGET_TRIPLE=""
EXTRA_CARGO_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --release)
            IS_RELEASE=true
            shift
            ;;
        --debug)
            IS_RELEASE=false
            shift
            ;;
        --out-dir)
            OUT_DIR="$2"
            shift 2
            ;;
        --target)
            TARGET_TRIPLE="$2"
            shift 2
            ;;
        *)
            EXTRA_CARGO_ARGS+=("$1")
            shift
            ;;
    esac
done

cd "$DESKTOP_DIR"
BUILD_FLAGS=("-p" "console-app")
TARGET_SUBDIR="debug"
if [[ "$IS_RELEASE" == true ]]; then
    BUILD_FLAGS+=("--release")
    TARGET_SUBDIR="release"
fi
if [[ -n "$TARGET_TRIPLE" ]]; then
    BUILD_FLAGS+=("--target" "$TARGET_TRIPLE")
fi

echo "==> Building Linux binary with cargo..."
if [[ ${#EXTRA_CARGO_ARGS[@]} -gt 0 ]]; then
    cargo build "${BUILD_FLAGS[@]}" "${EXTRA_CARGO_ARGS[@]}"
else
    cargo build "${BUILD_FLAGS[@]}"
fi

if [[ -n "$TARGET_TRIPLE" ]]; then
    BINARY_SRC="$DESKTOP_DIR/target/$TARGET_TRIPLE/$TARGET_SUBDIR/console"
else
    BINARY_SRC="$DESKTOP_DIR/target/$TARGET_SUBDIR/console"
fi
if [[ ! -f "$BINARY_SRC" ]]; then
    BINARY_SRC="$WORKSPACE_ROOT/target/$TARGET_SUBDIR/console"
fi
if [[ ! -f "$BINARY_SRC" ]]; then
    echo "Error: Could not locate compiled binary at $BINARY_SRC" >&2
    exit 1
fi

STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGE_DIR"' EXIT

cp "$BINARY_SRC" "$STAGE_DIR/console"
chmod +x "$STAGE_DIR/console"

ICON_PNG="$DESKTOP_DIR/assets/icon_transparent.png"
if [[ ! -f "$ICON_PNG" ]]; then
    ICON_PNG="$WORKSPACE_ROOT/apps/mobile/assets/icon.png"
fi
if [[ -f "$ICON_PNG" ]]; then
    cp "$ICON_PNG" "$STAGE_DIR/console.png"
fi

cat << 'DESKTOP' > "$STAGE_DIR/console.desktop"
[Desktop Entry]
Type=Application
Name=Console
Comment=Console Desktop
Exec=console
Icon=console
Terminal=false
Categories=Development;Utility;
StartupWMClass=Console
DESKTOP

mkdir -p "$OUT_DIR"
ARCHIVE="$OUT_DIR/Console-linux-x64.tar.gz"
tar -czf "$ARCHIVE" -C "$STAGE_DIR" .

echo "==> Done! Linux package created: $ARCHIVE"
