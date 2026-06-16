#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

log() {
  printf '[%s] [populate_site] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

cd "$SCRIPT_DIR/.."
if [ -f .env ]; then
  log "loading .env"
  set -a
  . ./.env
  set +a
fi
SITE_JS="${DAILY_NEWSLETTER_SITE_JS:-/usr/local/lib/daily-newsletter/populate_site.js}"
if [ -f "$SITE_JS" ]; then
  log "running site populator from $SITE_JS"
  node "$SITE_JS"
else
  BUILD_DIR="${TMPDIR:-/tmp}/daily_newsletter"
  log "compiling site populator to $BUILD_DIR"
  mkdir -p "$BUILD_DIR"
  tsc --target ES2020 --module commonjs --lib ES2020,DOM --skipLibCheck "$SCRIPT_DIR/populate_site.ts" --outDir "$BUILD_DIR"
  log "running compiled site populator"
  node "$BUILD_DIR/populate_site.js"
fi
log "site population completed"
