#!/usr/bin/env python3
"""Personal curated newsletter collector/generator/sender.

Output policy: no generated summaries. Newsletter contains source titles and links only.
"""
from __future__ import annotations

import argparse
import html
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
import urllib.parse
import ssl
from html.parser import HTMLParser
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

REPO = Path(os.environ.get("NEWSLETTER_REPO", str(Path.home() / "projects" / "daily_newsletter")))
SITE = REPO / "site"
RUNTIME = Path(os.environ.get("NEWSLETTER_RUNTIME", str(Path.home() / ".hermes" / "newsletter")))
SOURCES = SITE / "sources.json"
STATE = RUNTIME / "state.json"
OUTDIR = RUNTIME / "output"
USER_AGENT = "HermesPersonalNewsletter/1.0 (+https://hermes-agent.nousresearch.com)"
MAX_AGE_HOURS = 30

CRITICAL_WORDS = re.compile(
    r"\b(war|invasion|missile|bombing|ceasefire|shutdown|grève générale|fermeture|emergency|urgent|zero-day|outage|frontier model|gpt-[0-9]|claude [0-9]|election result|resigns|sanctions|earthquake|flood|terror|iran|gaza|ukraine)\b",
    re.I,
)
IMPORTANT_WORDS = re.compile(
    r"\b(model|agent|agents|benchmark|paper|research|architecture|postmortem|security|vulnerability|cve|attack|kubernetes|linux|barcelona|barça|injury|transfer|champions league|laliga|algérie|algeria|france|paris|ratp|metro|rer|consulat|visa|immigration|policy|law|élections|election)\b",
    re.I,
)
LOW_SIGNAL_WORDS = re.compile(
    r"\b(quiz|celebrity|horoscope|shopping|promo|sponsored|coupon|deal|gallery|photos?\b|video only|live updates)\b",
    re.I,
)


def load_json(path: Path, default: Any) -> Any:
    if not path.exists():
        return default
    return json.loads(path.read_text(encoding="utf-8"))


def save_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=25) as resp:
        return resp.read()


def text_of(el: ET.Element | None) -> str:
    if el is None or el.text is None:
        return ""
    return re.sub(r"\s+", " ", el.text).strip()


def attr_by_local_name(el: ET.Element, local_name: str) -> str:
    for key, value in el.attrib.items():
        if key.split("}")[-1].lower() == local_name.lower() and value:
            return value.strip()
    return ""


def first_image_url(el: ET.Element) -> str:
    # RSS/Atom image conventions: media:content, media:thumbnail, enclosure, image tags, and og:image in descriptions.
    for node in el.iter():
        local = node.tag.split("}")[-1].lower()
        url = attr_by_local_name(node, "url") or attr_by_local_name(node, "href")
        medium = attr_by_local_name(node, "medium").lower()
        mime = attr_by_local_name(node, "type").lower()
        if url and (local in {"thumbnail", "image"} or medium == "image" or mime.startswith("image/")):
            return html.unescape(url)
    for field in ["description", "content", "summary"]:
        txt = text_of(child(el, [field]))
        m = re.search(r'<img[^>]+src=["\']([^"\']+)', txt, re.I)
        if m:
            return html.unescape(m.group(1))
    return ""


def child(el: ET.Element, names: list[str]) -> ET.Element | None:
    for n in names:
        found = el.find(n)
        if found is not None:
            return found
    # namespace-insensitive fallback
    wanted = {n.split("}")[-1] for n in names}
    for c in list(el):
        if c.tag.split("}")[-1] in wanted:
            return c
    return None


def parse_date(value: str) -> str | None:
    if not value:
        return None
    value = value.strip()
    # Keep original if parsing fails; try common RSS/Atom shapes.
    for fmt in [
        "%a, %d %b %Y %H:%M:%S %z",
        "%a, %d %b %Y %H:%M:%S %Z",
        "%Y-%m-%dT%H:%M:%S%z",
        "%Y-%m-%dT%H:%M:%SZ",
        "%Y-%m-%d",
    ]:
        try:
            dt = datetime.strptime(value.replace("GMT", "+0000"), fmt)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=timezone.utc)
            return dt.astimezone(timezone.utc).isoformat()
        except Exception:
            pass
    return value


def parse_feed(raw: bytes, source: dict[str, Any], theme: str) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    root = ET.fromstring(raw)
    tag = root.tag.lower()
    channel_el = root.find("channel")
    if tag.endswith("rss") or channel_el is not None:
        channel = channel_el if channel_el is not None else root
        entries = channel.findall("item")
        for e in entries:
            title = text_of(child(e, ["title"]))
            link = text_of(child(e, ["link"]))
            guid = text_of(child(e, ["guid"])) or link or title
            published = parse_date(text_of(child(e, ["pubDate", "published", "updated"])))
            if title and link:
                items.append(make_item(title, link, guid, published, source, theme, first_image_url(e)))
    else:
        # Atom
        entries = [e for e in root.iter() if e.tag.split("}")[-1] == "entry"]
        for e in entries:
            title = text_of(child(e, ["title"]))
            link = ""
            for c in list(e):
                if c.tag.split("}")[-1] == "link":
                    link = c.attrib.get("href", "") or text_of(c)
                    if link:
                        break
            guid = text_of(child(e, ["id"])) or link or title
            published = parse_date(text_of(child(e, ["published", "updated"])))
            if title and link:
                items.append(make_item(title, link, guid, published, source, theme, first_image_url(e)))
    return items



class LinkExtractor(HTMLParser):
    """Tiny dependency-free link extractor for simple HTML source pages."""

    def __init__(self, base_url: str) -> None:
        super().__init__()
        self.base_url = base_url
        self.links: list[dict[str, str]] = []
        self._href: str | None = None
        self._text: list[str] = []
        self.og_image = ""

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        attr = {k.lower(): (v or "") for k, v in attrs}
        if tag.lower() == "meta":
            prop = (attr.get("property") or attr.get("name") or "").lower()
            if prop in {"og:image", "twitter:image"} and attr.get("content") and not self.og_image:
                self.og_image = urllib.parse.urljoin(self.base_url, attr["content"])
        if tag.lower() == "a" and attr.get("href"):
            self._href = urllib.parse.urljoin(self.base_url, html.unescape(attr["href"]))
            self._text = []

    def handle_data(self, data: str) -> None:
        if self._href:
            self._text.append(data)

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "a" and self._href:
            title = re.sub(r"\s+", " ", " ".join(self._text)).strip()
            if title:
                self.links.append({"title": title, "link": self._href})
            self._href = None
            self._text = []


def parse_html_source(raw: bytes, source: dict[str, Any], theme: str) -> list[dict[str, Any]]:
    """Parse configured HTML/website sources into title/link items.

    Supports optional source config:
    - include_url_regex: only keep matching links
    - exclude_url_regex: drop matching links
    - max_items: cap extracted links
    """
    base_url = source["url"]
    parser = LinkExtractor(base_url)
    parser.feed(raw.decode("utf-8", errors="replace"))
    include = source.get("include_url_regex")
    exclude = source.get("exclude_url_regex")
    max_items = int(source.get("max_items", 25))
    items: list[dict[str, Any]] = []
    seen: set[str] = set()
    for link in parser.links:
        href = link["link"].split("#", 1)[0]
        title = link["title"]
        if href in seen or href == base_url:
            continue
        if include and not re.search(str(include), href):
            continue
        if exclude and re.search(str(exclude), href):
            continue
        if len(title) < 8 or LOW_SIGNAL_WORDS.search(title):
            continue
        seen.add(href)
        items.append(make_item(title, href, href, None, source, theme, parser.og_image))
        if len(items) >= max_items:
            break
    return items


def parse_api_source(raw: bytes, source: dict[str, Any], theme: str) -> list[dict[str, Any]]:
    """Parse known JSON API sources; currently supports IDFM/RATP disruption feeds."""
    data = json.loads(raw.decode("utf-8", errors="replace"))
    items: list[dict[str, Any]] = []
    disruptions = data.get("disruptions") if isinstance(data, dict) else None
    if isinstance(disruptions, list):
        for disruption in disruptions[:50]:
            if not isinstance(disruption, dict):
                continue
            title = disruption.get("title") or disruption.get("cause") or disruption.get("id") or "Transport disruption"
            link = disruption.get("url") or source.get("homepage") or "https://www.iledefrance-mobilites.fr/"
            published = disruption.get("updated_at") or disruption.get("created_at") or None
            items.append(make_item(str(title), str(link), str(disruption.get("id") or link or title), parse_date(str(published or "")), source, theme))
    return items


def fetch_source(source: dict[str, Any]) -> bytes:
    headers = {"User-Agent": USER_AGENT}
    if source.get("auth") == "apiKey":
        key = os.environ.get("IDFM_API_KEY") or os.environ.get("RATP_API_KEY") or os.environ.get("PRIM_API_KEY")
        if not key:
            raise RuntimeError(f"{source['name']} requires IDFM_API_KEY/RATP_API_KEY/PRIM_API_KEY")
        headers["apikey"] = key
    req = urllib.request.Request(source["url"], headers=headers)
    context = ssl._create_unverified_context() if source.get("insecure_ssl") else None
    with urllib.request.urlopen(req, timeout=25, context=context) as resp:
        return resp.read()


def parse_source(raw: bytes, source: dict[str, Any], theme: str) -> list[dict[str, Any]]:
    kind = str(source.get("type", "rss")).lower()
    if kind in {"rss", "feed", "atom", "xml"}:
        return parse_feed(raw, source, theme)
    if kind in {"html", "website", "web"}:
        return parse_html_source(raw, source, theme)
    if kind == "api":
        return parse_api_source(raw, source, theme)
    raise ValueError(f"unsupported source type {kind!r} for {source.get('name')}")


def make_item(title: str, link: str, guid: str, published: str | None, source: dict[str, Any], theme: str, image: str = "") -> dict[str, Any]:
    title = html.unescape(re.sub(r"\s+", " ", re.sub(r"<[^>]+>", "", title))).strip()
    importance, score = score_item(title, source, theme)
    return {
        "id": guid or link or title,
        "title": title,
        "link": link.strip(),
        "theme": theme,
        "importance": importance,
        "source": source.get("name", "Unknown"),
        "source_tier": int(source.get("tier", 3)),
        "score": score,
        "published": published,
        "image": image.strip(),
        "first_seen": datetime.now(timezone.utc).isoformat(),
    }


def score_item(title: str, source: dict[str, Any], theme: str) -> tuple[str, int]:
    tier = int(source.get("tier", 3))
    score = tier * 3
    if CRITICAL_WORDS.search(title):
        score += 8
    if IMPORTANT_WORDS.search(title):
        score += 4
    if LOW_SIGNAL_WORDS.search(title):
        score -= 6
    if theme in {"AI / Agentic", "Software Engineering / Architecture"}:
        score += 7
    elif theme in {"France / Paris Local", "Algeria", "FC Barcelona"}:
        score += 2
    elif theme in {"Politics / Geopolitics", "General News"}:
        score -= 2
    if score >= 22:
        imp = "Critical"
    elif score >= 17:
        imp = "Important"
    elif score >= 13:
        imp = "Watch"
    else:
        imp = "Normal"
    return imp, score


def collect() -> dict[str, Any]:
    config = load_json(SOURCES, {})
    state = load_json(STATE, {"items": {}, "scan_errors": []})
    items_by_id: dict[str, Any] = state.setdefault("items", {})
    scan_errors: list[dict[str, str]] = []
    new_count = 0
    fetched_count = 0
    for theme_block in config.get("themes", []):
        theme = theme_block["theme"]
        for source in theme_block.get("sources", []):
            url = source["url"]
            try:
                raw = fetch_source(source)
                fetched_count += 1
                for item in parse_source(raw, source, theme):
                    key = item["link"] or item["id"]
                    if key not in items_by_id:
                        items_by_id[key] = item
                        new_count += 1
                    elif item.get("image") and not items_by_id[key].get("image"):
                        items_by_id[key]["image"] = item["image"]
            except Exception as e:
                scan_errors.append({"source": source.get("name", url), "url": url, "error": f"{type(e).__name__}: {e}"})
            time.sleep(0.2)
    state["last_scan"] = datetime.now(timezone.utc).isoformat()
    state["scan_errors"] = scan_errors[-100:]
    save_json(STATE, state)
    return {"fetched_sources": fetched_count, "new_items": new_count, "errors": len(scan_errors), "state": str(STATE)}


SPANISH_ONLY_SOURCES = {"Mundo Deportivo Barça", "Sport Barça"}
ALGERIA_FOOTBALL_SOURCES = {"DZfoot", "La Gazette du Fennec"}
TECH_HOME_THEMES = {"AI / Agentic", "Software Engineering / Architecture"}
LOW_HOME_THEMES = {"Politics / Geopolitics", "General News"}
PAPER_SOURCES = {"arXiv cs.AI", "arXiv cs.CL"}
FAMOUS_PAPER_HINTS = re.compile(
    r"\b(GPT-?[45]?|Claude|Gemini|Llama|Mistral|DeepMind|OpenAI|Anthropic|AlphaFold|AlphaGo|Sora|Imagen|Attention Is All You Need|Nature|Science|frontier model)\b",
    re.I,
)
SPANISH_HINTS = re.compile(r"\b(el|la|los|las|un|una|del|por|para|con|sin|sobre|tras|ante|barça|fichaje|jugador|equipo|temporada|mercado|oferta|salida|llega|sigue|nuevo|primera|última|así|está|años)\b", re.I)
FRENCH_HINTS = re.compile(r"\b(le|la|les|des|du|une|pour|avec|sans|sur|dans|plus|après|avant|algérie|france|paris|équipe|coupe|monde|jour|annonce)\b", re.I)


def allowed_language(item: dict[str, Any]) -> bool:
    """Allow only English, French, or Arabic. Conservative Spanish filter for known Barca sources."""
    source = str(item.get("source", ""))
    title = str(item.get("title", ""))
    if source in SPANISH_ONLY_SOURCES:
        return False
    if re.search(r"[\u0600-\u06FF]", title):
        return True
    # If title contains strong Spanish hints and no French hints, treat as non-allowed.
    if SPANISH_HINTS.search(title) and not FRENCH_HINTS.search(title):
        return False
    return True


def is_algeria_football(item: dict[str, Any]) -> bool:
    source = str(item.get("source", ""))
    title = str(item.get("title", ""))
    return source in ALGERIA_FOOTBALL_SOURCES or bool(re.search(r"\b(fennec|verts|mondial|coupe du monde|football|dzfoot|sélection|convocation)\b", title, re.I))


def is_paper(item: dict[str, Any]) -> bool:
    source = str(item.get("source", ""))
    title = str(item.get("title", ""))
    return source in PAPER_SOURCES or bool(re.search(r"\b(arxiv|paper|preprint)\b", title, re.I))


def is_famous_paper(item: dict[str, Any]) -> bool:
    if not is_paper(item):
        return True
    title = str(item.get("title", ""))
    # Conservative: only include papers that are clearly tied to a famous lab/model/topic.
    return bool(FAMOUS_PAPER_HINTS.search(title))


def recent_items() -> list[dict[str, Any]]:
    state = load_json(STATE, {"items": {}})
    cutoff = datetime.now(timezone.utc) - timedelta(hours=MAX_AGE_HOURS)
    out = []
    for item in state.get("items", {}).values():
        ts = item.get("published") or item.get("first_seen")
        keep = True
        if ts:
            try:
                dt = datetime.fromisoformat(str(ts).replace("Z", "+00:00"))
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=timezone.utc)
                keep = dt >= cutoff
            except Exception:
                keep = True
        if keep and item.get("source") == "LWN":
            keep = False
        # Recompute score at render time so policy changes affect existing collected items.
        item["importance"], item["score"] = score_item(item.get("title", ""), {"tier": item.get("source_tier", 3)}, item.get("theme", ""))
        if keep and item.get("score", 0) >= 13 and allowed_language(item) and is_famous_paper(item):
            out.append(item)
    # dedupe by normalized title
    seen_titles = set()
    deduped = []
    for item in sorted(out, key=lambda x: (x.get("score", 0), x.get("source_tier", 0)), reverse=True):
        norm = re.sub(r"\W+", " ", item["title"].lower()).strip()[:120]
        if norm in seen_titles:
            continue
        seen_titles.add(norm)
        deduped.append(item)
    return deduped


def item_key(item: dict[str, Any]) -> str:
    link = str(item.get("link", "")).strip().lower()
    if link:
        return link
    return re.sub(r"\W+", " ", str(item.get("title", "")).lower()).strip()[:160]


def select_items(items: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], list[dict[str, Any]], dict[str, list[dict[str, Any]]]]:
    """Select page sections with no duplicates and a tech/AI-heavy home page."""
    used: set[str] = set()

    # Home: informative tech/software/architecture/AI first. Politics/general only if exceptional and capped.
    home: list[dict[str, Any]] = []
    per_theme: dict[str, int] = {}

    def try_add_home(item: dict[str, Any], max_theme: int = 3) -> bool:
        key = item_key(item)
        if key in used or len(home) >= 5:
            return False
        theme = item.get("theme", "")
        if per_theme.get(theme, 0) >= max_theme:
            return False
        home.append(item)
        used.add(key)
        per_theme[theme] = per_theme.get(theme, 0) + 1
        return True

    for item in items:
        if item.get("theme") in TECH_HOME_THEMES:
            try_add_home(item, max_theme=4)
        if len(home) >= 5:
            break
    # Fallback only if there are fewer than 3 tech/AI/software items in the window.
    if len(home) < 3:
        for item in items:
            theme = item.get("theme")
            if theme in LOW_HOME_THEMES or theme in {"Football", "FC Barcelona", "Algeria"}:
                continue
            try_add_home(item, max_theme=1)
            if len(home) >= 3:
                break

    # Must Read: only items not already used in Home.
    must: list[dict[str, Any]] = []
    for item in items:
        key = item_key(item)
        if key in used:
            continue
        if item["importance"] == "Critical" and item.get("score", 0) >= 23:
            must.append(item)
            used.add(key)
        if len(must) >= 5:
            break

    # Themes: max 3 each, excluding Home and Must Read. Algeria caps football to avoid a football-only Algeria page.
    themes: dict[str, list[dict[str, Any]]] = {}
    algeria_football_count = 0
    for item in items:
        key = item_key(item)
        if key in used:
            continue
        theme = item["theme"]
        if theme == "Algeria" and is_algeria_football(item):
            if algeria_football_count >= 1:
                continue
            algeria_football_count += 1
        bucket = themes.setdefault(theme, [])
        if len(bucket) < 3:
            bucket.append(item)
            used.add(key)
    return home, must, themes


def importance_classes(importance: str) -> tuple[str, str, str]:
    return {
        "Critical": ("from-[#ff3b6b] via-[#ff2bb8] to-[#ff6a00]", "text-white bg-[#ff3b6b]", "border-[#ff4d6d]/60"),
        "Important": ("from-[#ff8a00] via-[#ff3b6b] to-[#9b5cff]", "text-black bg-[#ffd166]", "border-[#ff8a00]/60"),
        "Watch": ("from-[#4c8dff] via-[#8b5cf6] to-[#ff4bb8]", "text-white bg-[#4c8dff]", "border-[#4c8dff]/60"),
        "Normal": ("from-[#3a3a3a] to-[#1f1f1f]", "text-[#d8d8d8] bg-[#2a2a2a]", "border-white/10"),
    }.get(importance, ("from-[#3a3a3a] to-[#1f1f1f]", "text-[#d8d8d8] bg-[#2a2a2a]", "border-white/10"))


def item_html(item: dict[str, Any]) -> str:
    importance = html.escape(item["importance"])
    gradient, pill, border = importance_classes(item["importance"])
    image = str(item.get("image") or "").strip()
    link = html.escape(item["link"])
    if image.startswith(("http://", "https://")):
        visual = f"""
        <a href="{link}" class="relative block aspect-[16/9] overflow-hidden rounded-[1.45rem] border border-white/10 bg-[#242424]">
          <img src="{html.escape(image)}" alt="" loading="lazy" referrerpolicy="no-referrer" class="h-full w-full object-cover opacity-90 transition duration-500 group-hover:scale-[1.035] group-hover:opacity-100" onerror="this.style.display='none';this.parentElement.classList.add('fallback-gradient')">
          <div class="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/45 via-transparent to-transparent"></div>
        </a>
        """
    else:
        visual = f"""
        <a href="{link}" class="fallback-gradient relative block aspect-[16/9] overflow-hidden rounded-[1.45rem] border border-white/10 bg-gradient-to-br {gradient}">
          <div class="absolute inset-0 blur-2xl opacity-80 bg-gradient-to-br {gradient}"></div>
          <div class="absolute inset-0 bg-black/10"></div>
        </a>
        """
    return f"""
    <article class="group space-y-3">
      {visual}
      <div class="space-y-2 px-0.5">
        <div class="flex flex-wrap gap-1.5 text-[11px] leading-none">
          <span class="rounded-full border border-white/10 bg-white/[0.06] px-2.5 py-1.5 font-medium text-[#bdbdbd]">{html.escape(item['theme'])}</span>
          <span class="rounded-full px-2.5 py-1.5 font-semibold {pill}">{importance}</span>
          <span class="rounded-full border border-white/10 bg-white/[0.06] px-2.5 py-1.5 font-medium text-[#9c9c9c]">{html.escape(item['source'])}</span>
        </div>
        <a class="block text-[21px] font-semibold leading-tight tracking-[-0.035em] text-white transition group-hover:text-[#ff5ab3]" href="{link}">{html.escape(item['title'])}</a>
      </div>
    </article>
    """


def section_shell(section_id: str, title: str, eyebrow: str, content: str) -> str:
    return f"""
    <section id="{section_id}" class="space-y-5">
      <div class="space-y-1">
        <p class="text-sm font-semibold text-[#8c8c8c]">{eyebrow}</p>
        <h2 class="text-[42px] font-bold leading-none tracking-[-0.065em] text-white sm:text-6xl">{title}</h2>
      </div>
      <div class="grid gap-7">{content}</div>
    </section>
    """


def render_html() -> tuple[str, Path]:
    items = recent_items()
    home, must, themes = select_items(items)
    now = datetime.now().strftime("%Y-%m-%d %H:%M")
    home_html = "\n".join(item_html(i) for i in home) or "<p class='rounded-3xl bg-white/[0.06] p-5 text-[#9c9c9c]'>No high-value items selected.</p>"
    must_html = "\n".join(item_html(i) for i in must) or "<p class='rounded-3xl bg-white/[0.06] p-5 text-[#9c9c9c]'>No major must-read items today.</p>"
    theme_sections = []
    for theme, bucket in themes.items():
        cards = "\n".join(item_html(i) for i in bucket)
        slug = re.sub(r"[^a-z0-9]+", "-", theme.lower()).strip("-")
        theme_sections.append(f"""
        <section id="theme-{slug}" class="space-y-4">
          <div class="flex items-end justify-between gap-4">
            <h3 class="text-[30px] font-bold leading-none tracking-[-0.055em] text-white">{html.escape(theme)}</h3>
            <span class="rounded-full border border-white/10 bg-white/[0.06] px-3 py-1 text-xs font-medium text-[#9c9c9c]">{len(bucket)} picks</span>
          </div>
          <div class="grid gap-7">{cards}</div>
        </section>
        """)
    themes_html = "\n".join(theme_sections) or "<p class='rounded-3xl bg-white/[0.06] p-5 text-[#9c9c9c]'>No theme items selected.</p>"

    doc = f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Daily Newsletter — {html.escape(now)}</title>
<script src="https://cdn.tailwindcss.com"></script>
<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@500;600;700;800&display=swap" rel="stylesheet">
<script>tailwind.config = {{ theme: {{ extend: {{ fontFamily: {{ sans: ['Inter','system-ui','sans-serif'] }} }} }} }}</script>
<style>
body {{ font-family: Inter, system-ui, sans-serif; background:#181818; }}
.lovable-logo {{ background: conic-gradient(from 210deg,#ff6a00,#ff2bb8,#4c8dff,#ff6a00); }}
.tab-active {{ background:#f4f4f0; color:#1b1b1b; }}
.tab-inactive {{ background:transparent; color:#aaa; }}
.fallback-gradient::after {{ content:""; position:absolute; inset:-18%; background:radial-gradient(circle at 50% 76%,#ff6a00 0 24%,#ff2b8a 42%,#4c8dff 68%,transparent 82%); filter:blur(24px); opacity:.9; }}
</style>
</head><body class="min-h-screen bg-[#181818] text-white antialiased">
<main class="mx-auto min-h-screen max-w-[720px] bg-[#181818] px-5 pb-12 pt-7">
  <header class="sticky top-0 z-20 -mx-5 border-b border-white/[0.06] bg-[#181818]/92 px-5 pb-5 pt-3 backdrop-blur-xl">
    <div class="flex items-center justify-between gap-4">
      <a href="#home" class="flex items-center gap-2.5" aria-label="Daily Newsletter home">
        <span class="lovable-logo grid h-9 w-9 place-items-center rounded-[10px] shadow-[0_0_28px_rgba(255,43,184,.28)]"><span class="h-4 w-4 rounded-full bg-white/20 blur-[1px]"></span></span>
        <span class="text-[40px] font-extrabold leading-none tracking-[-0.07em] text-white">Lovable</span>
      </a>
      <div class="flex items-center gap-3">
        <a href="https://mehdibenfredj.github.io/daily_newsletter/" class="rounded-xl bg-[#f4f4f0] px-4 py-3 text-[16px] font-semibold leading-none text-[#1b1b1b]">Get started</a>
        <button class="relative h-9 w-9" aria-label="Menu"><span class="absolute left-1 top-2 h-0.5 w-7 bg-[#d9d9d9]"></span><span class="absolute left-1 top-4 h-0.5 w-5 bg-[#d9d9d9]"></span><span class="absolute left-1 top-6 h-0.5 w-7 bg-[#d9d9d9]"></span></button>
      </div>
    </div>
  </header>

  <section class="relative -mx-5 overflow-hidden px-5 pb-9 pt-28 text-center">
    <div class="absolute inset-x-[-12%] bottom-0 h-[420px] bg-[radial-gradient(circle_at_50%_78%,#ff6a00_0_22%,#ff2b8a_42%,#4c8dff_69%,transparent_82%)] blur-2xl opacity-95"></div>
    <div class="relative mx-auto max-w-[620px]">
      <h1 class="text-[44px] font-extrabold leading-[1.02] tracking-[-0.065em] text-white sm:text-6xl">Build something Lovable</h1>
      <p class="mx-auto mt-4 max-w-[420px] text-[22px] font-semibold leading-tight tracking-[-0.035em] text-[#b8b8b8]">Today’s signal, without the feed</p>
      <div class="mx-auto mt-10 rounded-[2rem] border border-black/70 bg-[#222] p-5 text-left shadow-[0_24px_80px_rgba(0,0,0,.45)] ring-1 ring-white/10">
        <p class="mb-10 text-[20px] font-medium tracking-[-0.03em] text-[#d7d7d7]">Ask Hermes for the best things worth reading today</p>
        <div class="flex items-center justify-between text-[#9d9d9d]"><span class="grid h-12 w-12 place-items-center rounded-full bg-white/[0.07] text-3xl font-light">+</span><span class="font-semibold">Build⌄</span><span>⌾</span><span class="grid h-12 w-12 place-items-center rounded-full bg-[#d8d8d8] text-xl text-[#181818]">↑</span></div>
      </div>
    </div>
  </section>

  <nav class="mb-8 mt-2 grid grid-cols-3 rounded-2xl border border-white/10 bg-white/[0.05] p-1" role="tablist" aria-label="Newsletter sections">
    <button type="button" data-tab="home" role="tab" aria-selected="true" class="tab-button tab-active rounded-xl px-3 py-3 text-sm font-bold transition">Home</button>
    <button type="button" data-tab="must" role="tab" aria-selected="false" class="tab-button tab-inactive rounded-xl px-3 py-3 text-sm font-bold transition">Must Read</button>
    <button type="button" data-tab="themes" role="tab" aria-selected="false" class="tab-button tab-inactive rounded-xl px-3 py-3 text-sm font-bold transition">Themes</button>
  </nav>

  <div class="grid gap-10">
    <div data-panel="home" role="tabpanel">{section_shell('home', 'Home', 'top picks', home_html)}</div>
    <div data-panel="must" role="tabpanel" hidden>{section_shell('must', 'Must Read', 'major only', must_html)}</div>
    <div data-panel="themes" role="tabpanel" hidden>
      <section id="themes" class="space-y-7">
        <div class="space-y-1"><p class="text-sm font-semibold text-[#8c8c8c]">max 3 each</p><h2 class="text-[42px] font-bold leading-none tracking-[-0.065em] text-white sm:text-6xl">Themes</h2></div>
        <div class="space-y-10">{themes_html}</div>
      </section>
    </div>
  </div>
  <footer class="py-10 text-center text-sm font-medium text-[#8c8c8c]">{html.escape(now)} · plain links only · no generated summaries</footer>
</main>
<script>
function setTab(name) {{
  document.querySelectorAll('[data-panel]').forEach(panel => {{ panel.hidden = panel.dataset.panel !== name; }});
  document.querySelectorAll('[data-tab]').forEach(button => {{
    const active = button.dataset.tab === name;
    button.setAttribute('aria-selected', active ? 'true' : 'false');
    button.classList.toggle('tab-active', active);
    button.classList.toggle('tab-inactive', !active);
  }});
  history.replaceState(null, '', '#' + name);
}}
document.querySelectorAll('[data-tab]').forEach(button => button.addEventListener('click', () => setTab(button.dataset.tab)));
const initial = location.hash.replace('#','');
if (['home','must','themes'].includes(initial)) setTab(initial);
</script>
</body></html>"""
    OUTDIR.mkdir(parents=True, exist_ok=True)
    path = OUTDIR / f"briefing-{datetime.now().strftime('%Y-%m-%d')}.html"
    path.write_text(doc, encoding="utf-8")
    return doc, path


def cmd_collect(args: argparse.Namespace) -> None:
    print(json.dumps(collect(), ensure_ascii=False, indent=2))


def cmd_generate(args: argparse.Namespace) -> None:
    if args.collect_first:
        collect()
    _, path = render_html()
    print(json.dumps({"html": str(path)}, ensure_ascii=False, indent=2))


def main() -> None:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("collect").set_defaults(func=cmd_collect)
    gen = sub.add_parser("generate")
    gen.add_argument("--collect-first", action="store_true")
    gen.set_defaults(func=cmd_generate)
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
