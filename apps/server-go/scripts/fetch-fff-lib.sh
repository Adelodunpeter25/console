#!/usr/bin/env bash
# Fetch the prebuilt fff C-ABI shared library for this platform from the
# fff releases (https://github.com/dmtrKovalenko/fff). Already vendored for
# linux-x64; run this for other platforms.
set -euo pipefail
arch=$(uname -m)
os=$(uname -s)
case "$os" in
  Linux) target="${arch}-unknown-linux-gnu.so" ;;
  Darwin) target="${arch}-apple-darwin.dylib" ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac
mkdir -p "$(dirname "$0")/../third_party/fff"
curl -fL "https://github.com/dmtrKovalenko/fff/releases/latest/download/c-lib-${target}" \
  -o "$(dirname "$0")/../third_party/fff/libfff_c.so"
echo "fff c library installed to third_party/fff/libfff_c.so"
