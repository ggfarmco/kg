#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-batch-context.sh <files-json-path> <source-id>" >&2
  exit 2
fi

input="$1"
source_id="$2"

if [ ! -f "$input" ]; then
  echo "input file not found: $input" >&2
  exit 2
fi

jq -c '.[]' "$input" | while read -r file; do
  fileId=$(echo "$file" | jq -r '.node_id')
  decls=$(kg node children "$fileId" \
    | jq --arg src "$source_id" '
      [.data[] | select(.layer == "decl") | {
        node_id: .id,
        name: .name,
        kind: (.properties[$src].kind // ""),
        line_range: [
          (.properties[$src].line_start // 0),
          (.properties[$src].line_end   // 0)
        ]
      }]
    ')
  echo "$file" | jq --argjson decls "$decls" '. + {decls: $decls}'
done | jq -s '.'
