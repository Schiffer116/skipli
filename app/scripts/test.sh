#!/usr/bin/env bash
# Manual curl smoke tests against a running dev server (see scripts/dev.sh).
set -euo pipefail

HOST="${HOST:-localhost:3000}"

usage() {
  cat <<EOF
Usage:
  scripts/test.sh create <owner> <name> [description]
  scripts/test.sh list <owner>

Env:
  HOST   server address (default: localhost:3000)
EOF
  exit 1
}

cmd="${1:-}"
case "$cmd" in
  create)
    owner="${2:?owner required}"
    name="${3:?name required}"
    description="${4:-}"
    curl -sS -X POST "$HOST/api/boards" \
      -H 'Content-Type: application/json' \
      -d "$(printf '{"owner":"%s","name":"%s","description":"%s"}' "$owner" "$name" "$description")" \
      -w '\n%{http_code}\n'
    ;;
  list)
    owner="${2:?owner required}"
    curl -sS -X GET "$HOST/api/boards" \
      -H 'Content-Type: application/json' \
      -d "$(printf '{"owner":"%s"}' "$owner")" \
      -w '\n%{http_code}\n'
    ;;
  *)
    usage
    ;;
esac
