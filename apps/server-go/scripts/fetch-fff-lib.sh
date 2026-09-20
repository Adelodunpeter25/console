#!/usr/bin/env bash
# Fetch the prebuilt fff C-ABI shared library for this platform from the fff
# releases (https://github.com/dmtrKovalenko/fff). Supported: Linux x64/arm64,
# macOS x64/arm64. Run once per machine; the binary is not committed.
set -euo pipefail
arch="$(uname -m)"
os="$(uname -s)"
case "$os" in
  Linux)
    case "$arch" in
      x86_64) asset="c-lib-x86_64-unknown-linux-gnu.so" ;;
      aarch64|arm64) asset="c-lib-aarch64-unknown-linux-gnu.so" ;;
      *) echo "unsupported linux arch: $arch" >&2; exit 1 ;;
    esac
    out="libfff_c.so"
    ;;
  Darwin)
    case "$arch" in
      x86_64) asset="c-lib-x86_64-apple-darwin.dylib" ;;
      arm64) asset="c-lib-aarch64-apple-darwin.dylib" ;;
      *) echo "unsupported macOS arch: $arch" >&2; exit 1 ;;
    esac
    out="libfff_c.dylib"
    ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac
dest="$(cd "$(dirname "$0")/.." && pwd)/third_party/fff"
mkdir -p "$dest"
curl -fL "https://github.com/dmtrKovalenko/fff/releases/latest/download/${asset}" \
  -o "$dest/$out"
echo "fff c library installed to third_party/fff/$out"
