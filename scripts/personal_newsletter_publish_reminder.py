#!/usr/bin/env python3
"""Collect, publish the daily newsletter, and print a Telegram-friendly reminder.

No email is sent from this workflow.
"""
from __future__ import annotations

import importlib.util
import os
import subprocess
from pathlib import Path

REPO = Path(os.environ.get("NEWSLETTER_REPO", str(Path.home() / "projects" / "daily_newsletter")))
NEWSLETTER_SCRIPT = REPO / "scripts" / "personal_newsletter.py"
PUBLISH_SCRIPT = REPO / "scripts" / "publish_newsletter.py"
DEFAULT_PUBLIC_URL = "https://mehdibenfredj.github.io/daily_newsletter/"


def load_newsletter_module():
    spec = importlib.util.spec_from_file_location("personal_newsletter", NEWSLETTER_SCRIPT)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {NEWSLETTER_SCRIPT}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def run_publish() -> None:
    subprocess.run(
        ["python3", str(PUBLISH_SCRIPT)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        cwd=str(REPO),
    )


def main() -> None:
    run_publish()

    mod = load_newsletter_module()
    items = mod.recent_items()
    # Reminder must-reads should be true major items from the latest candidate set;
    # page rendering separately guarantees no duplicates across sections.
    must = [i for i in items if i["importance"] == "Critical" and i.get("score", 0) >= 23][:5]
    public_url = os.environ.get("NEWSLETTER_PUBLIC_URL", DEFAULT_PUBLIC_URL)

    lines = ["🗞️ Daily newsletter is ready", "", f"Link: {public_url}", "", "Must reads:"]
    if must:
        for item in must[:5]:
            lines.append(f"- [{item['importance']}] {item['title']} — {item['source']}")
            lines.append(f"  {item['link']}")
    else:
        lines.append("- No major must-read items today.")

    print("\n".join(lines))


if __name__ == "__main__":
    main()
