#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
"$SCRIPT_DIR/run_go_publisher.sh" "$@"
"$SCRIPT_DIR/populate_site.sh"
