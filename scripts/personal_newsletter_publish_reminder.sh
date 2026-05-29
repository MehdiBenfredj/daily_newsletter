#!/usr/bin/env bash
set -euo pipefail
cd "${NEWSLETTER_REPO:-$HOME/projects/daily_newsletter}"
set -a
[ -f "$HOME/.hermes/.env" ] && . "$HOME/.hermes/.env"
set +a
python3 scripts/personal_newsletter_publish_reminder.py
