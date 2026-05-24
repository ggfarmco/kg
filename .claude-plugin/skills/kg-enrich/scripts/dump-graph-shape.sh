#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-graph-shape.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

packages=$(kg node list --domain "$domain" --layer package --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      slug: .id,
      name: .name,
      path: (.properties[$src].path // ""),
      files: []
    })
  ')

files=$(kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      package_node_id: .parent_id,
      path: (.properties[$src].path // ""),
      name: .name
    })
  ')

packages=$(jq -n --argjson pkgs "$packages" --argjson files "$files" '
  $pkgs | map(. as $p | $p + {
    files: ($files | map(select(.package_node_id == $p.slug)) | map({node_id, path, name}))
  })
')

imports='[]'
for fid in $(echo "$files" | jq -r '.[].node_id'); do
  edges=$(kg edge list-from "$fid" --type imports 2>/dev/null \
    | jq --arg fid "$fid" 'if .data then [.data[] | {from: $fid, to: .target_id}] else [] end' \
    || echo '[]')
  imports=$(jq -n --argjson cur "$imports" --argjson new "$edges" '$cur + $new')
done

jq -n \
  --argjson packages "$packages" \
  --argjson imports "$imports" \
  '{packages: $packages, imports: $imports}'
