#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
if command -v publish-newsletter >/dev/null 2>&1; then
  publish-newsletter "$@"
else
  go run ./cmd/publish-newsletter "$@"
fi
