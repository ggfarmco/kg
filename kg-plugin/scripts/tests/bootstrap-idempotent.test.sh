#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

PLUGIN_ROOT=$(mktemp -d)
FAKE_HOME=$(mktemp -d)
trap 'rm -rf "$PLUGIN_ROOT" "$FAKE_HOME"' EXIT

mkdir -p "$PLUGIN_ROOT/.claude-plugin" "$PLUGIN_ROOT/scripts"
echo '{"name":"kg","version":"0.3.1"}' > "$PLUGIN_ROOT/.claude-plugin/plugin.json"
cp ../bootstrap.sh "$PLUGIN_ROOT/scripts/"

export HOME="$FAKE_HOME"
export KG_HOME="$FAKE_HOME/.config/kg"
export CLAUDE_PLUGIN_ROOT="$PLUGIN_ROOT"

mkdir -p "$KG_HOME/bin" "$KG_HOME/extractor-plugins/tree-sitter"
printf '#!/bin/sh\necho kg\n' > "$KG_HOME/bin/kg"
chmod +x "$KG_HOME/bin/kg"
echo "v0.3.1" > "$KG_HOME/VERSION"

out=$(bash "$PLUGIN_ROOT/scripts/bootstrap.sh" 2>&1)

grep -q "already installed" <<< "$out" \
  || { echo "FAIL idempotent: expected 'already installed', got: $out"; exit 1; }
grep -q "Downloading" <<< "$out" \
  && { echo "FAIL idempotent: bootstrap tried to download despite matching VERSION"; exit 1; }

echo "OK bootstrap-idempotent.sh"
