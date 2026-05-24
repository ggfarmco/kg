#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node list --domain myapp --layer file --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-files.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-files.sh myapp tree-sitter:0.2.0)
expected=$(cat fixtures/expected-dump-files.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-files.sh"; exit 1; }
echo "OK dump-files.sh"
