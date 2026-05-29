#!/usr/bin/env bash
set -euo pipefail
cd "$HOME/projects/daily_newsletter"
python3 scripts/publish_newsletter.py
