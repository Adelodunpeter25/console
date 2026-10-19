#!/usr/bin/env bash
# Build the WebTransport client for Android and stage it into the APK.
#
# Plan Phase 3 spike. `cargo-ndk` would do the toolchain wiring, but the NDK
# clang wrappers are enough and keep the dependency count down, so the tool
# paths are set here.
#
# Produces:
#   app/src/main/jniLibs/arm64-v8a/libconsole_wt_android.so
#   app/src/main/jniLibs/x86_64/libconsole_wt_android.so
#
# Those copies are gitignored: a binary in the repo is not a reviewable
# artifact, and it is reproducible from this script.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_JNI="$SCRIPT_DIR/../app/src/main/jniLibs"

NDK_VERSION="${ANDROID_NDK_VERSION:-28.2.13676358}"
NDK="${ANDROID_NDK_HOME:-${ANDROID_HOME:-$HOME/Library/Android/sdk}/ndk/$NDK_VERSION}"
BIN="$NDK/toolchains/llvm/prebuilt/darwin-x86_64/bin"
if [[ ! -d "$BIN" ]]; then
    # The host directory is named per-host (`linux-x86_64` on a CI runner, and
    # `darwin-x86_64` / `darwin-arm64` depending on the Mac), so take whichever
    # the NDK ships instead of the name written above. An empty result falls
    # through to the check below, which says what is missing.
    HOST_DIR="$(find "$NDK/toolchains/llvm/prebuilt" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | head -1 || true)"
    BIN="${HOST_DIR:-$NDK/toolchains/llvm/prebuilt}/bin"
fi
if [[ ! -x "$BIN/aarch64-linux-android21-clang" ]]; then
    echo "error: NDK clang wrappers not found under $BIN" >&2
    exit 1
fi

# ring (wtransport's TLS backend) is C + asm, so its build script needs the
# same NDK clang — not just the final link. The NDK ships `llvm-ar` only,
# hence the explicit AR overrides. The names contain dashes, which `export`
# rejects, so they go through `env` per build.
build_target() {
    local target="$1"
    # Must run inside the crate: `.cargo/config.toml` (the NDK linker) is
    # discovered from the working directory, not from --manifest-path.
    #
    # That config's linker paths name the host they were written on, so the
    # wrapper found above is passed through CARGO_TARGET_*_LINKER as well.
    # Environment beats config file, which is what lets the same script run on
    # a Linux CI runner, where the committed darwin paths do not exist.
    (
        cd "$SCRIPT_DIR"
        env \
            "CC_aarch64-linux-android=$BIN/aarch64-linux-android21-clang" \
            "CC_x86_64-linux-android=$BIN/x86_64-linux-android21-clang" \
            "AR_aarch64-linux-android=$BIN/llvm-ar" \
            "AR_x86_64-linux-android=$BIN/llvm-ar" \
            "CARGO_TARGET_AARCH64_LINUX_ANDROID_LINKER=$BIN/aarch64-linux-android21-clang" \
            "CARGO_TARGET_X86_64_LINUX_ANDROID_LINKER=$BIN/x86_64-linux-android21-clang" \
            cargo build --release --target "$target"
    )
}

for pair in "aarch64-linux-android:arm64-v8a" "x86_64-linux-android:x86_64"; do
    target="${pair%%:*}"
    abi="${pair##*:}"
    echo "==> building $target"
    build_target "$target"
    mkdir -p "$APP_JNI/$abi"
    cp "$SCRIPT_DIR/target/$target/release/libconsole_wt_android.so" "$APP_JNI/$abi/"
    echo "    staged $(du -h "$APP_JNI/$abi/libconsole_wt_android.so" | cut -f1) for $abi"
done

echo "==> done"