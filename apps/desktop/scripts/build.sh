#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE_ROOT="$(cd "$DESKTOP_DIR/../.." && pwd)"

MODE="dev"
IS_RELEASE=false
BUNDLE_ID=""
APP_NAME=""
OUT_DIR="$DESKTOP_DIR/dist"
EXTRA_CARGO_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)
            MODE="$2"
            shift 2
            ;;
        --release)
            IS_RELEASE=true
            MODE="prod"
            shift
            ;;
        --debug)
            IS_RELEASE=false
            MODE="dev"
            shift
            ;;
        --bundle-id)
            BUNDLE_ID="$2"
            shift 2
            ;;
        --app-name)
            APP_NAME="$2"
            shift 2
            ;;
        --out-dir)
            OUT_DIR="$2"
            shift 2
            ;;
        *)
            EXTRA_CARGO_ARGS+=("$1")
            shift
            ;;
    esac
done

if [[ -z "$BUNDLE_ID" ]]; then
    if [[ "$MODE" == "prod" ]]; then
        BUNDLE_ID="com.console.mobile.prod"
    else
        BUNDLE_ID="com.console.mobile.dev"
    fi
fi

if [[ -z "$APP_NAME" ]]; then
    if [[ "$MODE" == "prod" ]]; then
        APP_NAME="Console"
    else
        APP_NAME="Console Dev"
    fi
fi

echo "========================================="
echo " Packaging: $APP_NAME ($MODE)"
echo " Bundle ID: $BUNDLE_ID"
echo " Output   : $OUT_DIR/$APP_NAME.app"
echo "========================================="

# 1. Generate macOS AppIcon.icns if needed
ASSETS_DIR="$DESKTOP_DIR/assets"
ICNS_FILE="$ASSETS_DIR/AppIcon.icns"
ICON_PNG="$WORKSPACE_ROOT/apps/mobile/assets/icon.png"

mkdir -p "$ASSETS_DIR"

if [[ ! -f "$ICNS_FILE" && -f "$ICON_PNG" ]]; then
    echo "==> Generating AppIcon.icns from mobile icon.png..."
    ICONSET_DIR="$(mktemp -d)/AppIcon.iconset"
    mkdir -p "$ICONSET_DIR"

    sips -z 16 16     "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16.png" > /dev/null 2>&1
    sips -z 32 32     "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16@2x.png" > /dev/null 2>&1
    sips -z 32 32     "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32.png" > /dev/null 2>&1
    sips -z 64 64     "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32@2x.png" > /dev/null 2>&1
    sips -z 128 128   "$ICON_PNG" --out "$ICONSET_DIR/icon_128x128.png" > /dev/null 2>&1
    sips -z 256 256   "$ICON_PNG" --out "$ICONSET_DIR/icon_128x128@2x.png" > /dev/null 2>&1
    sips -z 256 256   "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256.png" > /dev/null 2>&1
    sips -z 512 512   "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256@2x.png" > /dev/null 2>&1
    sips -z 512 512   "$ICON_PNG" --out "$ICONSET_DIR/icon_512x512.png" > /dev/null 2>&1
    sips -z 1024 1024 "$ICON_PNG" --out "$ICONSET_DIR/icon_512x512@2x.png" > /dev/null 2>&1

    iconutil -c icns "$ICONSET_DIR" -o "$ICNS_FILE"
    rm -rf "$(dirname "$ICONSET_DIR")"
fi

# 2. Build the binary
cd "$DESKTOP_DIR"
BUILD_FLAGS=("-p" "console-app")
TARGET_SUBDIR="debug"

if [[ "$IS_RELEASE" == true || "$MODE" == "prod" ]]; then
    BUILD_FLAGS+=("--release")
    TARGET_SUBDIR="release"
fi

echo "==> Building binary with cargo..."
if [[ ${#EXTRA_CARGO_ARGS[@]} -gt 0 ]]; then
    cargo build "${BUILD_FLAGS[@]}" "${EXTRA_CARGO_ARGS[@]}"
else
    cargo build "${BUILD_FLAGS[@]}"
fi

BINARY_SRC="$DESKTOP_DIR/target/$TARGET_SUBDIR/console"
if [[ ! -f "$BINARY_SRC" ]]; then
    BINARY_SRC="$WORKSPACE_ROOT/target/$TARGET_SUBDIR/console"
fi

if [[ ! -f "$BINARY_SRC" ]]; then
    echo "Error: Could not locate compiled binary at $BINARY_SRC" >&2
    exit 1
fi

# 3. Create .app bundle structure
APP_DIR="$OUT_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "==> Assembling .app bundle at $APP_DIR..."
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

# Copy binary
cp "$BINARY_SRC" "$MACOS_DIR/console"
chmod +x "$MACOS_DIR/console"

# Copy icon
if [[ -f "$ICNS_FILE" ]]; then
    cp "$ICNS_FILE" "$RESOURCES_DIR/AppIcon.icns"
fi

# 4. Generate Info.plist
VERSION=$(grep '^version' "$DESKTOP_DIR/Cargo.toml" | head -n 1 | cut -d '"' -f 2 || echo "0.1.0")

cat << PLIST > "$CONTENTS_DIR/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>${BUNDLE_ID}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleExecutable</key>
    <string>console</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
</dict>
</plist>
PLIST

# 5. Ad-hoc codesign
echo "==> Code-signing bundle (ad-hoc)..."
codesign --force --deep --sign - "$APP_DIR"

echo "========================================="
echo "==> Done! App bundle created:"
echo "    $APP_DIR"
echo "========================================="
