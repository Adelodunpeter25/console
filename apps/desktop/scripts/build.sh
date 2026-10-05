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
TARGET_TRIPLE=""
WITH_CEF=false
SIGN_IDENTITY="-"
MAIN_ENTITLEMENTS=""
HELPER_ENTITLEMENTS=""
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
        # Cross-compile triple (e.g. x86_64-apple-darwin on an arm64 host).
        # The binary lands under target/<triple>/, handled in section 2.
        --target)
            TARGET_TRIPLE="$2"
            shift 2
            ;;
        # Chromium (CEF) browser backend: build with the `cef-browser` cargo
        # feature and assemble the framework + helper apps into the bundle
        # (section 4b). Requires CEF_PATH pointing at a shared CEF download
        # (see scripts/dev.sh); dev.sh exports a default.
        --cef)
            WITH_CEF=true
            EXTRA_CARGO_ARGS+=("--features" "cef-browser")
            shift
            ;;
        # Signing identity (`-` = ad-hoc, the default). Pass a Developer ID
        # for distribution together with the entitlements flags below.
        --sign)
            SIGN_IDENTITY="$2"
            shift 2
            ;;
        --main-entitlements)
            MAIN_ENTITLEMENTS="$2"
            shift 2
            ;;
        --helper-entitlements)
            HELPER_ENTITLEMENTS="$2"
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
        BUNDLE_ID="com.console.desktop"
    else
        BUNDLE_ID="com.console.desktop.dev"
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
ICON_PNG="$ASSETS_DIR/icon_transparent.png"
if [[ ! -f "$ICON_PNG" ]]; then
    ICON_PNG="$WORKSPACE_ROOT/apps/mobile/assets/icon.png"
fi

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
if [[ -n "$TARGET_TRIPLE" ]]; then
    BUILD_FLAGS+=("--target" "$TARGET_TRIPLE")
fi
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

# 4b. CEF framework + helper apps (only with --cef)
# Layout mirrors cef-rs `bundle-cef-app` (see `cef::build_util::mac`):
#   Contents/Frameworks/Chromium Embedded Framework.framework
#   Contents/Frameworks/<bin> Helper{, (GPU),(Renderer),(Plugin),(Alerts)}.app
# each helper a full .app whose executable is a copy of our own binary (CEF
# re-executes it for renderer/GPU/... subprocesses; see runtime.rs).
if [[ "$WITH_CEF" == true ]]; then
    echo "==> Assembling CEF framework and helpers..."
    CEF_DIST="${CEF_PATH:-$HOME/.local/share/cef}"
    case "$(uname -m)" in
        x86_64) CEF_ARCH="x86_64" ;;
        arm64) CEF_ARCH="aarch64" ;;
        *) echo "Error: unsupported architecture $(uname -m) for CEF" >&2; exit 1 ;;
    esac
    # Exactly one CEF version directory must match (unexpanded globs fail the
    # existence check below, which also keeps this safe under `set -u` on the
    # system bash 3.2).
    CEF_CANDIDATES=( "$CEF_DIST"/*/cef_macos_"$CEF_ARCH" )
    if [[ ${#CEF_CANDIDATES[@]} -ne 1 || ! -e "${CEF_CANDIDATES[0]}" ]]; then
        echo "Error: expected exactly one CEF distribution at $CEF_DIST/*/cef_macos_$CEF_ARCH" >&2
        exit 1
    fi
    CEF_FW_SRC="${CEF_CANDIDATES[0]}/Chromium Embedded Framework.framework"
    if [[ ! -d "$CEF_FW_SRC" ]]; then
        echo "Error: CEF framework missing at $CEF_FW_SRC" >&2
        exit 1
    fi

    FRAMEWORKS_DIR="$CONTENTS_DIR/Frameworks"
    mkdir -p "$FRAMEWORKS_DIR"
    echo "    Copying framework (one-time cost, ~330MB)..."
    rm -rf "$FRAMEWORKS_DIR/Chromium Embedded Framework.framework"
    ditto "$CEF_FW_SRC" "$FRAMEWORKS_DIR/Chromium Embedded Framework.framework"

    BIN_STEM="$(basename "$MACOS_DIR/console")"
    HELPER_BASE="$BIN_STEM Helper"
    for SUFFIX in "" " (GPU)" " (Renderer)" " (Plugin)" " (Alerts)"; do
        HELPER_NAME="$HELPER_BASE$SUFFIX"
        HELPER_APP="$FRAMEWORKS_DIR/$HELPER_NAME.app"
        rm -rf "$HELPER_APP"
        mkdir -p "$HELPER_APP/Contents/MacOS" "$HELPER_APP/Contents/Resources"
        cp "$MACOS_DIR/console" "$HELPER_APP/Contents/MacOS/$HELPER_NAME"
        chmod +x "$HELPER_APP/Contents/MacOS/$HELPER_NAME"
        SLUG=$(echo "$SUFFIX" | tr -d ' ()' | tr '[:upper:]' '[:lower:]')
        if [[ -n "$SLUG" ]]; then
            HELPER_ID="$BUNDLE_ID.helper.$SLUG"
        else
            HELPER_ID="$BUNDLE_ID.helper"
        fi
        cat << PLIST > "$HELPER_APP/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>${HELPER_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>${HELPER_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>${HELPER_ID}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleExecutable</key>
    <string>${HELPER_NAME}</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
PLIST
    done
    echo "    Helpers installed: $HELPER_BASE{, (GPU),(Renderer),(Plugin),(Alerts)}"
fi

# 5. Code signing, inside-out so nested code is sealed before the container.
# Ad-hoc (`-`) by default; pass --sign with a Developer ID for distribution.
echo "==> Code-signing bundle ($SIGN_IDENTITY)..."
sign() {
    if [[ -n "$2" ]]; then
        codesign --force --sign "$SIGN_IDENTITY" --entitlements "$2" "$1"
    else
        codesign --force --sign "$SIGN_IDENTITY" "$1"
    fi
}
if [[ "$WITH_CEF" == true ]]; then
    sign "$FRAMEWORKS_DIR/Chromium Embedded Framework.framework" ""
    for HELPER_APP in "$FRAMEWORKS_DIR"/*.app; do
        sign "$HELPER_APP" "$HELPER_ENTITLEMENTS"
    done
fi
sign "$APP_DIR" "$MAIN_ENTITLEMENTS"

echo "========================================="
echo "==> Done! App bundle created:"
echo "    $APP_DIR"
echo "========================================="
