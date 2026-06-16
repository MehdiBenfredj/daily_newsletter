#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

log() {
  printf '[%s] [run_go_publisher] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

cd "$SCRIPT_DIR/.."
if command -v publish-newsletter >/dev/null 2>&1; then
  log "running installed publish-newsletter"
  publish-newsletter "$@"
else
  log "running Go publisher from source"
  go run ./cmd/publish-newsletter "$@"
fi
log "publisher completed"
