#!/usr/bin/env bash
set -euo pipefail
config=$(cat)
domain=$(echo "$config" | jq -r '.domain')
cat <<EOF
{
  "protocol_version": 2,
  "source": "bash-demo:0.2.0",
  "domain": "$domain",
  "scope": "domain-source",
  "domain_spec": {
    "id": "$domain",
    "layers": ["root", "item"],
    "description": "bash-demo declarative output"
  },
  "nodes": [
    {"id": "$domain:demo",        "layer": "root", "name": "Demo"},
    {"id": "$domain:demo-first",  "layer": "item", "parent": "$domain:demo", "name": "First"},
    {"id": "$domain:demo-second", "layer": "item", "parent": "$domain:demo", "name": "Second"}
  ],
  "edges": [
    {"src": "$domain:demo-first", "target": "$domain:demo-second", "type": "references"}
  ]
}
EOF
