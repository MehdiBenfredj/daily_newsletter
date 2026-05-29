#!/usr/bin/env python3
"""Publish generated Hermes newsletter HTML into the GitHub Pages repo."""
from __future__ import annotations

import json
import re
import shutil
import subprocess
from datetime import datetime
from pathlib import Path

HOME = Path.home()
REPO = HOME / "projects" / "daily_newsletter"
SITE = REPO / "site"
ARCHIVE = SITE / "archive"
SOURCE_OUT = HOME / ".hermes" / "newsletter" / "output"
GENERATOR = HOME / ".hermes" / "scripts" / "personal_newsletter.py"


def run(cmd: list[str], cwd: Path | None = None) -> str:
    return subprocess.check_output(cmd, cwd=str(cwd) if cwd else None, text=True).strip()


def main() -> None:
    run(["python3", str(GENERATOR), "generate"])
    today = datetime.now().strftime("%Y-%m-%d")
    src = SOURCE_OUT / f"briefing-{today}.html"
    if not src.exists():
        raise SystemExit(f"missing generated newsletter: {src}")

    SITE.mkdir(parents=True, exist_ok=True)
    ARCHIVE.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, SITE / "index.html")
    shutil.copy2(src, ARCHIVE / f"{today}.html")
    (SITE / ".nojekyll").touch()

    archive_path = SITE / "archive.json"
    existing = []
    if archive_path.exists():
        try:
            existing = json.loads(archive_path.read_text())
        except Exception:
            existing = []
    by_date: dict[str, dict[str, str]] = {}
    for x in existing:
        if isinstance(x, dict) and isinstance(x.get("date"), str):
            by_date[x["date"]] = {"date": str(x.get("date")), "path": str(x.get("path", "")), "title": str(x.get("title", ""))}
    by_date[today] = {"date": today, "path": f"archive/{today}.html", "title": f"Personal Briefing — {today}"}
    archive = [by_date[d] for d in sorted(by_date.keys(), reverse=True)]
    archive_path.write_text(json.dumps(archive, indent=2) + "\n")

    status = run(["git", "status", "--short"], cwd=REPO)
    if not status:
        print(json.dumps({"published": False, "reason": "no changes", "repo": str(REPO)}))
        return

    run(["git", "add", "README.md", ".github/workflows/pages.yml", "site"], cwd=REPO)
    run(["git", "commit", "-m", f"Publish newsletter {today}"], cwd=REPO)
    run(["git", "push", "-u", "origin", "main"], cwd=REPO)
    print(json.dumps({"published": True, "date": today, "repo": str(REPO), "html": str(SITE / "index.html")}))


if __name__ == "__main__":
    main()
