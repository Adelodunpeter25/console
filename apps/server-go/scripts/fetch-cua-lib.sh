#!/usr/bin/env bash
# Fetch the Cua Driver SDK shared library from trycua/cua releases.
#
# The server dlopens this at runtime (never linked, never committed).
# Default fetches the host platform into third_party/cua/, which the loader
# finds with no configuration. --all fetches macOS and Linux (both arches)
# into per-platform subdirs for release staging, so adding Linux support
# later needs no new research.
#
# Sources are the per-platform npm packages published on the driver release:
# each contains exactly package/libcua_driver_sdk.{dylib,so,dll}. They are
# smaller than the full CLI tarballs and their contents are confirmed.
# Windows packages exist too (win32-x64-msvc, win32-arm64-msvc) and slot into
# the table below when needed.
#
# Pinned to the driver version the server binds against; the runtime ABI
# check refuses anything incompatible, so drift fails loudly, not silently.
set -euo pipefail

VERSION="0.34.0"
BASE="https://github.com/trycua/cua/releases/download/cua-driver-rs-v${VERSION}"
# platform label, release asset, library file
PLATFORMS=(
  "darwin-arm64 trycua-cua-driver-darwin-arm64-${VERSION}.tgz libcua_driver_sdk.dylib"
  "darwin-x64 trycua-cua-driver-darwin-x64-${VERSION}.tgz libcua_driver_sdk.dylib"
  "linux-x64 trycua-cua-driver-linux-x64-gnu-${VERSION}.tgz libcua_driver_sdk.so"
  "linux-arm64 trycua-cua-driver-linux-arm64-gnu-${VERSION}.tgz libcua_driver_sdk.so"
)

DEST_ROOT="$(cd "$(dirname "$0")/.." && pwd)/third_party/cua"

fetch_one() {
  local label="$1" asset="$2" lib="$3" destdir="$4"
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' RETURN
  echo "fetching $asset..." >&2
  curl --retry 3 -fL "${BASE}/${asset}" -o "$tmpdir/pkg.tgz"
  tar -xzf "$tmpdir/pkg.tgz" -C "$tmpdir" "package/$lib"
  mkdir -p "$destdir"
  mv "$tmpdir/package/$lib" "$destdir/$lib"
  echo "installed $destdir/$lib"
}

host_label() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin)
      case "$arch" in
        arm64|aarch64) printf 'darwin-arm64' ;;
        x86_64) printf 'darwin-x64' ;;
        *) echo "unsupported macOS arch: $arch" >&2; exit 1 ;;
      esac
      ;;
    Linux)
      case "$arch" in
        x86_64|amd64) printf 'linux-x64' ;;
        aarch64|arm64) printf 'linux-arm64' ;;
        *) echo "unsupported linux arch: $arch" >&2; exit 1 ;;
      esac
      ;;
    *) echo "unsupported os: $os (macOS and Linux only for now)" >&2; exit 1 ;;
  esac
}

if [[ "${1:-}" == "--all" ]]; then
  for entry in "${PLATFORMS[@]}"; do
    # shellcheck disable=SC2086
    set -- $entry
    fetch_one "$1" "$2" "$3" "$DEST_ROOT/$1"
  done
  exit 0
fi

want="$(host_label)"
for entry in "${PLATFORMS[@]}"; do
  # shellcheck disable=SC2086
  set -- $entry
  if [[ "$1" == "$want" ]]; then
    fetch_one "$1" "$2" "$3" "$DEST_ROOT"
    exit 0
  fi
done
echo "no Cua library for this platform" >&2
exit 1
