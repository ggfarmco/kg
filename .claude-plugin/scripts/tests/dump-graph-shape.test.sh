#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node list --domain myapp --layer package --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-packages.json ;;
    *"node list --domain myapp --layer file --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-files-shape.json ;;
    *"edge list-from myapp:cmd/main-go --type imports"*)
      cat fixtures/kg-edge-from-main.json ;;
    *"edge list-from myapp:internal-handler/serve-go --type imports"*)
      cat fixtures/kg-edge-from-serve.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-graph-shape.sh myapp tree-sitter:0.2.0)
expected=$(cat fixtures/expected-graph-shape.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-graph-shape.sh"; exit 1; }
echo "OK dump-graph-shape.sh"
