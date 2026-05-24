#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-topology.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

files=$(kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      name: .name,
      path: (.properties[$src].path // "")
    })
  ')

entries=$(echo "$files" | jq '
  map(. + {
    score: (
      ([
        (if (.path | test("/main\\.go$")) then 5 else 0 end),
        (if (.path | test("/cmd/[^/]+/main\\.go$")) then 3 else 0 end)
      ] | add)
    )
  })
  | map(select(.score > 0))
  | sort_by(-.score)
')

imports='[]'
for fid in $(echo "$files" | jq -r '.[].node_id'); do
  edges=$(kg edge list-from "$fid" --type imports 2>/dev/null \
    | jq --arg fid "$fid" 'if .data then [.data[] | {from: $fid, to: .target_id}] else [] end' \
    || echo '[]')
  imports=$(jq -n --argjson cur "$imports" --argjson new "$edges" '$cur + $new')
done

fanned=$(jq -n --argjson files "$files" --argjson imports "$imports" '
  $files | map(. as $f |
    $f + {
      fan_in:  ([$imports[] | select(.to == $f.node_id)]  | length),
      fan_out: ([$imports[] | select(.from == $f.node_id)] | length)
    }
  )
')

hotspots=$(echo "$fanned" | jq 'sort_by(-.fan_in) | .[0:10]')

jq -n \
  --argjson entries  "$entries" \
  --argjson hotspots "$hotspots" \
  --argjson edges    "$imports" \
  '{entries: $entries, hotspots: $hotspots, edges: $edges}'
