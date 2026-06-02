#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$(realpath "$0")")"
python3 scripts/new_workflow/publish_newsletter.py
