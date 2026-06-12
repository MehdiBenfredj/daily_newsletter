#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$SCRIPT_DIR/../.."
BUILD_DIR="${TMPDIR:-/tmp}/daily_newsletter_new_workflow"
mkdir -p "$BUILD_DIR"
tsc --target ES2020 --module commonjs --lib ES2020,DOM --skipLibCheck scripts/new_workflow/populate_site.ts --outDir "$BUILD_DIR"
node "$BUILD_DIR/populate_site.js"
