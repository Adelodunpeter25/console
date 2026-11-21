#!/usr/bin/env bash
# Stage the prebuilt libvoxl shared library for this platform from the pinned
# voxl-go module. Supported: Linux x64/arm64, macOS x64/arm64.
set -euo pipefail

VOXL_MOD="github.com/Adelodunpeter25/voxl-go@v0.4.1-0.20261121122719-bf233246a93f"

arch="$(uname -m)"
os="$(uname -s)"
case "$os" in
  Linux)
    case "$arch" in
      x86_64) triple="x86_64-unknown-linux-gnu" ;;
      aarch64|arm64) triple="aarch64-unknown-linux-gnu" ;;
      *) echo "unsupported linux arch: $arch" >&2; exit 1 ;;
    esac
    out="libvoxl.so"
    ;;
  Darwin)
    case "$arch" in
      x86_64) triple="x86_64-apple-darwin" ;;
      arm64) triple="aarch64-apple-darwin" ;;
      *) echo "unsupported macOS arch: $arch" >&2; exit 1 ;;
    esac
    out="libvoxl.dylib"
    ;;
  *)
    echo "voxl is not supported on $os; skipping" >&2
    exit 0
    ;;
esac

server_dir="$(cd "$(dirname "$0")/.." && pwd)"
voxl_dir="$(go -C "$server_dir" mod download -json "$VOXL_MOD" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Dir"])')"
src="$voxl_dir/libs/$triple/$out"
if [ ! -f "$src" ]; then
  echo "libvoxl not found at $src" >&2
  exit 1
fi

dest="$server_dir/third_party/voxl"
mkdir -p "$dest"
cp "$src" "$dest/$out"
chmod 644 "$dest/$out"
if [ "$os" = "Darwin" ]; then
  codesign --force --sign - "$dest/$out" >/dev/null 2>&1 || true
fi
echo "voxl library installed to third_party/voxl/$out"
