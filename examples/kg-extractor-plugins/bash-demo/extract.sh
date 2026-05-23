#!/usr/bin/env bash
set -euo pipefail
config=$(cat)
domain=$(echo "$config" | jq -r '.domain')
cat <<EOF
{"op":"meta","args":{"plugin":"bash-demo","version":"0.1.0","total_ops":5}}
{"op":"domain.add","args":{"id":"$domain","layers":["root","item"],"if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"root","name":"Demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"First","id":"demo-first","parent":"$domain:demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"Second","id":"demo-second","parent":"$domain:demo","if_not_exists":true}}
{"op":"edge.add","args":{"source":"$domain:demo-first","target":"$domain:demo-second","type":"references","if_not_exists":true}}
EOF
