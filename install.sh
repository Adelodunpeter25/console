#!/bin/sh
# Console — install script
#
# Downloads the latest console (CLI) and console-server binaries from the
# rolling GitHub release and installs them into ~/.local/bin.
#
# Usage:
#   curl -fsSL <raw-url>/install.sh | sh
#
# Override the install directory with:
#   CONSOLE_INSTALL_DIR=/usr/local/bin sh install.sh

set -e

REPO="Adelodunpeter25/console"
BASE_URL="https://github.com/${REPO}/releases/download/console-server"

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Linux) os="linux" ;;
  Darwin) os="macos" ;;
  *)
    echo "Unsupported OS: $OS (only Linux and macOS are supported)" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64 | amd64) arch="x64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

SUFFIX="${os}-${arch}"
PREFIX="${CONSOLE_INSTALL_DIR:-$HOME/.local/bin}"

echo "Installing Console (${SUFFIX}) to ${PREFIX}..."

mkdir -p "$PREFIX"

curl -fSL "${BASE_URL}/console-${SUFFIX}" -o "${PREFIX}/console"
curl -fSL "${BASE_URL}/console-server-${SUFFIX}" -o "${PREFIX}/console-server"
chmod +x "${PREFIX}/console" "${PREFIX}/console-server"

case ":$PATH:" in
  *":${PREFIX}:"*) ;;
  *)
    echo ""
    echo "NOTE: ${PREFIX} is not in your PATH."
    echo "Add this to your shell profile (~/.zshrc or ~/.bashrc):"
    echo "  export PATH=\"\$PATH:${PREFIX}\""
    ;;
esac

echo ""
echo "✅ Installed. Try it:"
echo "   console start --port 3000   # start the agent daemon"
echo "   console status              # check if it's running"
echo "   console stop                # stop it"
