#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ASSETS_DIR="$(cd "$SCRIPT_DIR/../assets" && pwd)"

SOURCE_IMAGE="${1:-/Users/adelodunpeter/.gemini/antigravity-cli/brain/00df29d5-dbfc-4cc2-bb18-6869b4b72719/simplified_prism_clean_1787225536979.jpg}"

if [ ! -f "$SOURCE_IMAGE" ]; then
  echo "Error: Source image not found at $SOURCE_IMAGE"
  exit 1
fi

echo "==> Generating mobile icons from: $SOURCE_IMAGE"
mkdir -p "$ASSETS_DIR"

# 1. Main app icon (1024x1024)
sips -s format png -z 1024 1024 "$SOURCE_IMAGE" --out "$ASSETS_DIR/icon.png"

# 2. Android Adaptive icon foreground (1024x1024)
sips -s format png -z 1024 1024 "$SOURCE_IMAGE" --out "$ASSETS_DIR/android-icon-foreground.png"

# 3. Android Adaptive icon background (1024x1024)
sips -s format png -z 1024 1024 "$SOURCE_IMAGE" --out "$ASSETS_DIR/android-icon-background.png"

# 4. Android Adaptive icon monochrome (1024x1024)
sips -s format png -z 1024 1024 "$SOURCE_IMAGE" --out "$ASSETS_DIR/android-icon-monochrome.png"

# 5. Splash screen icon (1024x1024)
sips -s format png -z 1024 1024 "$SOURCE_IMAGE" --out "$ASSETS_DIR/splash-icon.png"

# 6. Web favicon (192x192)
sips -s format png -z 192 192 "$SOURCE_IMAGE" --out "$ASSETS_DIR/favicon.png"

echo "==> Successfully generated all mobile icon assets in $ASSETS_DIR"
ls -la "$ASSETS_DIR"
