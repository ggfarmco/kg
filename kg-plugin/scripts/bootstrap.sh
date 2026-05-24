#!/usr/bin/env bash
set -euo pipefail

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
KG_HOME="${KG_HOME:-$HOME/.config/kg}"
REPO_OWNER="ggfarmco"
REPO_NAME="kg"

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  *) echo "unsupported OS: $(uname -s) (supported: darwin, linux)" >&2; exit 2 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64)        ARCH=amd64 ;;
  *) echo "unsupported arch: $(uname -m) (supported: arm64, amd64)" >&2; exit 2 ;;
esac
PLATFORM="${OS}_${ARCH}"

if [ "$OS" = "darwin" ] && [ "$ARCH" = "amd64" ]; then
  echo "Intel macOS is not supported by v0.3.1+ releases. Build from source: https://github.com/ggfarmco/kg#developer-setup" >&2
  exit 2
fi

if ! command -v jq >/dev/null; then
  echo "jq is required to read plugin manifest. Install: brew install jq / apt install jq" >&2
  exit 2
fi

VERSION=$(jq -r '.version' "$PLUGIN_ROOT/.claude-plugin/plugin.json" 2>/dev/null || echo "")
if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
  echo "cannot read plugin.json version at $PLUGIN_ROOT/.claude-plugin/plugin.json" >&2
  exit 2
fi
TAG="v${VERSION#v}"

if [ -f "$KG_HOME/VERSION" ] \
   && [ "$(cat "$KG_HOME/VERSION")" = "$TAG" ] \
   && [ -x "$KG_HOME/bin/kg" ]; then
  echo "kg $TAG already installed at $KG_HOME"
  exit 0
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

TARBALL="kg_${TAG}_${PLATFORM}.tar.gz"
BASE_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$TAG"

echo "Downloading $BASE_URL/$TARBALL ..."
curl -fL --proto '=https' -o "$TMP/$TARBALL" "$BASE_URL/$TARBALL"
curl -fL --proto '=https' -o "$TMP/checksums.txt" "$BASE_URL/checksums.txt"

EXPECTED=$(grep " ${TARBALL}\$" "$TMP/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "checksum entry missing for $TARBALL in checksums.txt" >&2
  exit 1
fi

if command -v sha256sum >/dev/null; then
  ACTUAL=$(sha256sum "$TMP/$TARBALL" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "$TMP/$TARBALL" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "checksum mismatch for $TARBALL: expected $EXPECTED, got $ACTUAL" >&2
  echo "report at https://github.com/$REPO_OWNER/$REPO_NAME/issues" >&2
  exit 1
fi

mkdir -p "$KG_HOME/bin" "$KG_HOME/extractor-plugins/tree-sitter"

EXTRACT="$TMP/extract"
mkdir -p "$EXTRACT"
tar -xzf "$TMP/$TARBALL" -C "$EXTRACT"

for b in kg kg-extractor kg-extractor-tree-sitter; do
  if [ ! -f "$EXTRACT/$b" ]; then
    echo "tarball missing expected binary: $b" >&2
    exit 1
  fi
  mv -f "$EXTRACT/$b" "$KG_HOME/bin/$b"
  chmod +x "$KG_HOME/bin/$b"
done

if [ ! -f "$EXTRACT/manifest.json" ]; then
  echo "tarball missing expected manifest.json" >&2
  exit 1
fi
mv -f "$EXTRACT/manifest.json" "$KG_HOME/extractor-plugins/tree-sitter/manifest.json"

ln -sf "$KG_HOME/bin/kg-extractor-tree-sitter" \
       "$KG_HOME/extractor-plugins/tree-sitter/kg-extractor-tree-sitter"

echo "$TAG" > "$KG_HOME/VERSION"

echo "kg $TAG installed to $KG_HOME"
echo "Optional: export PATH=\"$KG_HOME/bin:\$PATH\""
