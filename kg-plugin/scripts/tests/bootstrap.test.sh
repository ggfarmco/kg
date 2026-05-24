#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

PLUGIN_ROOT=$(mktemp -d)
FAKE_HOME=$(mktemp -d)
STUB=$(mktemp -d)
SHIM=$(mktemp -d)
trap 'rm -rf "$PLUGIN_ROOT" "$FAKE_HOME" "$STUB" "$SHIM"' EXIT

mkdir -p "$PLUGIN_ROOT/.claude-plugin" "$PLUGIN_ROOT/scripts"
echo '{"name":"kg","version":"0.3.1"}' > "$PLUGIN_ROOT/.claude-plugin/plugin.json"
cp ../bootstrap.sh "$PLUGIN_ROOT/scripts/"

export HOME="$FAKE_HOME"
export KG_HOME="$FAKE_HOME/.config/kg"
export CLAUDE_PLUGIN_ROOT="$PLUGIN_ROOT"

for b in kg kg-extractor kg-extractor-tree-sitter; do
  printf '#!/bin/sh\necho %s "$@"\n' "$b" > "$STUB/$b"
  chmod +x "$STUB/$b"
done
echo '{"name":"tree-sitter"}' > "$STUB/manifest.json"
echo 'fake readme'  > "$STUB/README.md"
echo 'fake license' > "$STUB/LICENSE"
tar -C "$STUB" -czf "$STUB/tarball.tar.gz" \
  kg kg-extractor kg-extractor-tree-sitter manifest.json README.md LICENSE

if command -v sha256sum >/dev/null; then
  EXPECTED_HASH=$(sha256sum "$STUB/tarball.tar.gz" | awk '{print $1}')
else
  EXPECTED_HASH=$(shasum -a 256 "$STUB/tarball.tar.gz" | awk '{print $1}')
fi

OS=$(uname -s | tr 'A-Z' 'a-z')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
TARBALL_NAME="kg_v0.3.1_${OS}_${ARCH}.tar.gz"
echo "$EXPECTED_HASH  $TARBALL_NAME" > "$STUB/checksums.txt"

cat > "$SHIM/curl" <<EOF
#!/usr/bin/env bash
out=""
prev=""
for arg; do
  if [ "\$prev" = "-o" ]; then out="\$arg"; fi
  prev="\$arg"
done
url="\${@: -1}"
case "\$url" in
  *"$TARBALL_NAME") cp "$STUB/tarball.tar.gz" "\$out" ;;
  *"checksums.txt") cp "$STUB/checksums.txt" "\$out" ;;
  *) echo "unexpected curl URL: \$url" >&2; exit 1 ;;
esac
EOF
chmod +x "$SHIM/curl"
export PATH="$SHIM:$PATH"

bash "$PLUGIN_ROOT/scripts/bootstrap.sh" >/dev/null

fail() { echo "FAIL bootstrap.sh: $1"; exit 1; }

[ -x "$KG_HOME/bin/kg" ]                                              || fail "kg not installed"
[ -x "$KG_HOME/bin/kg-extractor" ]                                    || fail "kg-extractor not installed"
[ -x "$KG_HOME/bin/kg-extractor-tree-sitter" ]                        || fail "kg-extractor-tree-sitter not installed"
[ -f "$KG_HOME/extractor-plugins/tree-sitter/manifest.json" ]         || fail "manifest not in extractor-plugins/"
[ -L "$KG_HOME/extractor-plugins/tree-sitter/kg-extractor-tree-sitter" ] || fail "symlink missing"
[ "$(cat "$KG_HOME/VERSION")" = "v0.3.1" ]                            || fail "VERSION file wrong"

[ ! -e "$KG_HOME/bin/README.md"     ] || fail "README leaked into bin/"
[ ! -e "$KG_HOME/bin/LICENSE"       ] || fail "LICENSE leaked into bin/"
[ ! -e "$KG_HOME/bin/manifest.json" ] || fail "manifest leaked into bin/"

echo "OK bootstrap.sh"
