#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

export TRACE_FILE
TRACE_FILE=$(mktemp)
kg() {
  echo "$*" > "$TRACE_FILE"
  echo '{"ok":true,"data":{"nodes_added":0}}'
}
export -f kg

echo '{"protocol_version":2,"source":"kg-summary:0.1.0","domain":"myapp","scope":"additive","nodes":[],"edges":[]}' \
  | ../apply-snapshot.sh kg-summary:0.1.0 myapp additive >/dev/null

got=$(cat "$TRACE_FILE")
expected="apply --source kg-summary:0.1.0 --domain myapp --scope additive"
[ "$got" = "$expected" ] || { echo "FAIL apply-snapshot.sh: got '$got' expected '$expected'"; exit 1; }
echo "OK apply-snapshot.sh"
rm -f "$TRACE_FILE"
