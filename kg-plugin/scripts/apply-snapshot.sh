#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "usage: apply-snapshot.sh <source-id> <domain-id> <scope>" >&2
  echo "snapshot JSON is read from stdin." >&2
  exit 2
fi

kg apply --source "$1" --domain "$2" --scope "$3"
