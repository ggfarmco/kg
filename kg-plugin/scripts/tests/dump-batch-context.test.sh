#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node children myapp:graph/handler-go"*)
      cat fixtures/kg-node-children-handler.json ;;
    *"node children myapp:graph/service-go"*)
      cat fixtures/kg-node-children-service.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-batch-context.sh fixtures/dump-batch-input.json tree-sitter:0.2.0)
expected=$(cat fixtures/expected-dump-batch.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-batch-context.sh"; exit 1; }
echo "OK dump-batch-context.sh"
