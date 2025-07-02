#!/usr/bin/env bash
set -euo pipefail

REPO="vi66r/kumiki-cli"
VERSION="${VERSION:-latest}"
ARCH="$(uname -m)"
OS="$(uname | tr '[:upper:]' '[:lower:]')"

# --------------------------- Fetch latest version ----------------------------
if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name":' | cut -d'"' -f4)"
fi

echo "📥  Installing Kumiki ${VERSION} …"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

TARBALL="kumiki_${OS}_${ARCH}.tar.gz"

curl -sSL "https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}" \
  -o "${TMPDIR}/${TARBALL}"

tar -xzf "${TMPDIR}/${TARBALL}" -C "${TMPDIR}"
chmod +x "${TMPDIR}/kumiki"

sudo mv "${TMPDIR}/kumiki" /usr/local/bin/kumiki
sudo ln -sf /usr/local/bin/kumiki /usr/local/bin/km

echo "✅  kumiki → /usr/local/bin/kumiki (alias: km)"

# --------------------------- Ensure XcodeGen ---------------------------------
if ! command -v xcodegen >/dev/null 2>&1; then
  echo "📦  XcodeGen not found. Installing via Homebrew…"
  if command -v brew >/dev/null 2>&1; then
    brew install xcodegen
  else
    echo "❌  Homebrew not installed; please install XcodeGen manually."
    exit 1
  fi
fi

echo "🎉  Install complete. Run 'km new' and you can just do things."
