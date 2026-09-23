#!/usr/bin/env bash
# Hot-reload dev server: restarts `go run ./cmd/skipli` whenever a .go file changes.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

find . -name '*.go' -not -path './node_modules/*' | entr -rn go run ./cmd/skipli
