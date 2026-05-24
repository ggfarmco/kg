#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-files.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      file_path: (.properties[$src].path // ""),
      package_node_id: .parent_id,
      name: .name
    })
  '
