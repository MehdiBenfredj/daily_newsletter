#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
SITE_DIR="$REPO_ROOT/site"
ARCHIVE_DATE="$(date +%F)"
COMMIT_DATE="$(date +%y-%m-%d)"
PROCESSED_ARCHIVE_DIR="$SITE_DIR/archives/processed"
UI_ARCHIVE_DIR="$SITE_DIR/archives/ui"
PROCESSED_ARCHIVE_PATH="$PROCESSED_ARCHIVE_DIR/$ARCHIVE_DATE.json"
UI_ARCHIVE_PATH="$UI_ARCHIVE_DIR/$ARCHIVE_DATE.html"
ARCHIVE_JSON_PATH="$SITE_DIR/archive.json"

mkdir -p "$PROCESSED_ARCHIVE_DIR" "$UI_ARCHIVE_DIR"

"$SCRIPT_DIR/run_go_publisher.sh" "$@"
"$SCRIPT_DIR/populate_site.sh"

cp "$REPO_ROOT/processed_informations.json" "$PROCESSED_ARCHIVE_PATH"
cp "$SITE_DIR/index.html" "$UI_ARCHIVE_PATH"

ARCHIVE_DATE="$ARCHIVE_DATE" ARCHIVE_JSON_PATH="$ARCHIVE_JSON_PATH" UI_ARCHIVE_PATH="$UI_ARCHIVE_PATH" PROCESSED_ARCHIVE_PATH="$PROCESSED_ARCHIVE_PATH" node <<'NODE'
const fs = require("fs");
const path = require("path");

const archiveDate = process.env.ARCHIVE_DATE;
const archiveJsonPath = process.env.ARCHIVE_JSON_PATH;
const siteDir = path.dirname(archiveJsonPath);

function relativeToSite(filePath) {
  return path.relative(siteDir, filePath).split(path.sep).join("/");
}

let archive = [];
if (fs.existsSync(archiveJsonPath)) {
  archive = JSON.parse(fs.readFileSync(archiveJsonPath, "utf8"));
}

const nextEntry = {
  date: archiveDate,
  path: relativeToSite(process.env.UI_ARCHIVE_PATH),
  processed_path: relativeToSite(process.env.PROCESSED_ARCHIVE_PATH),
  title: `Personal Briefing \u2014 ${archiveDate}`,
};

archive = archive.filter((entry) => entry.date !== archiveDate);
archive.unshift(nextEntry);
archive.sort((a, b) => b.date.localeCompare(a.date));

fs.writeFileSync(archiveJsonPath, `${JSON.stringify(archive, null, 2)}\n`);
NODE

git -C "$REPO_ROOT" add site
if git -C "$REPO_ROOT" diff --cached --quiet -- site; then
  echo "No site changes to commit."
else
  git -C "$REPO_ROOT" commit -m "daily_newsletter_$COMMIT_DATE" -- site
  git -C "$REPO_ROOT" push
fi
