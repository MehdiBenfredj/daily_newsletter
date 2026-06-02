#!/usr/bin/env python3
"""Publish generated Hermes newsletter HTML into the GitHub Pages repo."""
from __future__ import annotations

import argparse
import html
import json
import os
import re
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from http.client import IncompleteRead, RemoteDisconnected
from pathlib import Path
from typing import Any


REPO = Path(__file__).resolve().parents[2]
DEFAULT_SOURCES = REPO / "site" / "sources.json"
USER_AGENT = "daily-newsletter/1.0 (+https://github.com/)"
FETCH_TIMEOUT_SECONDS = 45
FETCH_RETRIES = 3


def collect_from_sources(path: Path) -> dict[str, Any]:
    sources_config = json.loads(path.read_text(encoding="utf-8"))
    themes = sources_config.get("themes", [])
    if not isinstance(themes, list):
        raise ValueError(f"expected 'themes' to be a list in {path}")

    sources: list[dict[str, object]] = []
    for theme_entry in themes:
        if not isinstance(theme_entry, dict):
            continue

        theme = theme_entry.get("theme")
        theme_sources = theme_entry.get("sources", [])
        if not isinstance(theme, str) or not isinstance(theme_sources, list):
            continue

        for source in theme_sources:
            if not isinstance(source, dict):
                continue

            sources.append(
                {
                    "theme": theme,
                    "name": source.get("name"),
                    "url": source.get("url"),
                    "type": source.get("type", "rss"),
                    "tier": source.get("tier"),
                    "config": source,
                }
            )

    return {
        "source_path": str(path),
        "theme_count": len(themes),
        "source_count": len(sources),
        "sources": sources,
    }


def fetch_source_data(source: dict[str, Any]) -> bytes:
    headers = {
        "User-Agent": USER_AGENT,
        "Accept": "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.7",
        "Accept-Encoding": "identity",
        "Connection": "close",
    }
    config = source.get("config", {})
    if isinstance(config, dict) and config.get("auth") == "apiKey":
        key = os.environ.get(
            "PRIM_API_KEY") or "hoSW8gEMbrG0m44ikXcXKmTYeXMBk2xV"
        if not key:
            raise RuntimeError(
                f"{source.get('name')} requires IDFM_API_KEY/RATP_API_KEY/PRIM_API_KEY")
        headers["apikey"] = key

    source_url = source.get("url")
    if not isinstance(source_url, str) or not source_url:
        raise ValueError(f"{source.get('name')} is missing a URL")
    fallback_url = config.get("fallback_url") if isinstance(
        config, dict) else None
    if fallback_url is not None and not isinstance(fallback_url, str):
        raise ValueError(f"{source.get('name')} fallback_url must be a string")

    context = None
    if isinstance(config, dict) and config.get("insecure_ssl"):
        context = ssl._create_unverified_context()

    url = source_url
    last_error: Exception | None = None
    for attempt in range(FETCH_RETRIES):
        request = urllib.request.Request(url, headers=headers)
        try:
            return fetch_with_context(request, context)
        except urllib.error.HTTPError as exc:
            last_error = exc
            if exc.code == 403 and fallback_url and url != fallback_url:
                url = fallback_url
                continue
            raise
        except urllib.error.URLError as exc:
            last_error = exc
            if context is None and isinstance(exc.reason, ssl.SSLCertVerificationError):
                context = ssl._create_unverified_context()
        except (IncompleteRead, RemoteDisconnected, TimeoutError, ConnectionError) as exc:
            last_error = exc

        if attempt < FETCH_RETRIES - 1:
            time.sleep(2**attempt)

    if last_error:
        raise last_error
    raise RuntimeError(f"failed to fetch {url}")


def fetch_with_context(request: urllib.request.Request, context: ssl.SSLContext | None) -> bytes:
    with urllib.request.urlopen(request, timeout=FETCH_TIMEOUT_SECONDS, context=context) as response:
        return response.read()


def process_rss_source(source: dict[str, Any]) -> dict[str, Any]:
    raw = fetch_source_data(source)
    return {
        "content_type": "rss",
        "bytes": len(raw),
        "data": raw.decode("utf-8", errors="replace"),
    }


def process_website_source(source: dict[str, Any]) -> dict[str, Any]:
    raw = fetch_source_data(source)
    return {
        "content_type": "website",
        "bytes": len(raw),
        "data": raw.decode("utf-8", errors="replace"),
    }


def process_api_source(source: dict[str, Any]) -> dict[str, Any]:
    raw = fetch_source_data(source)
    text = raw.decode("utf-8", errors="replace")
    try:
        data: Any = json.loads(text)
    except json.JSONDecodeError:
        data = text

    return {
        "content_type": "api",
        "bytes": len(raw),
        "data": data,
    }


def process_source(source: dict[str, Any]) -> dict[str, Any]:
    source_type = str(source.get("type", "rss")).lower()
    if source_type in {"rss", "feed", "atom", "xml"}:
        return process_rss_source(source)
    if source_type in {"website", "html", "web"}:
        return process_website_source(source)
    if source_type == "api":
        return process_api_source(source)
    raise ValueError(
        f"unsupported source type {source_type!r} for {source.get('name')}")


def process_sources(data: dict[str, Any]) -> dict[str, Any]:
    processed_sources: list[dict[str, Any]] = []
    errored_sources: list[dict[str, Any]] = []
    sources = data.get("sources", [])
    if not isinstance(sources, list):
        raise ValueError("expected collected data to contain a 'sources' list")

    for source in sources:
        if not isinstance(source, dict):
            continue

        result = {
            "theme": source.get("theme"),
            "name": source.get("name"),
            "url": source.get("url"),
            "type": source.get("type", "rss"),
            "config": source.get("config", {}),
        }
        try:
            result["processed"] = process_source(source)
            result["ok"] = True
        except Exception as exc:
            result["ok"] = False
            result["error"] = str(exc)
            errored_sources.append(
                {
                    "theme": source.get("theme"),
                    "name": source.get("name"),
                    "url": source.get("url"),
                    "type": source.get("type", "rss"),
                    "error": str(exc),
                }
            )
        processed_sources.append(result)

    return {
        **data,
        "processed_source_count": len(processed_sources),
        "errored_source_count": len(errored_sources),
        "errored_sources": errored_sources,
        "processed_sources": processed_sources,
    }


def xml_text(element: ET.Element | None) -> str:
    if element is None or element.text is None:
        return ""
    return re.sub(r"\s+", " ", html.unescape(element.text)).strip()


def xml_child(element: ET.Element, names: list[str]) -> ET.Element | None:
    wanted = {name.split("}")[-1].lower() for name in names}
    for child in list(element):
        if child.tag.split("}")[-1].lower() in wanted:
            return child
    return None


def parse_rss_information(processed: dict[str, Any]) -> list[dict[str, Any]]:
    raw_data = processed.get("data")
    if not isinstance(raw_data, str):
        return []

    root = ET.fromstring(raw_data)
    channel = root.find("channel")
    entries = channel.findall("item") if channel is not None else []
    if not entries:
        entries = [entry for entry in root.iter() if entry.tag.split("}")
                   [-1].lower() == "entry"]

    information: list[dict[str, Any]] = []
    for entry in entries:
        link = xml_text(xml_child(entry, ["link"]))
        if not link:
            for child in list(entry):
                if child.tag.split("}")[-1].lower() == "link":
                    link = child.attrib.get("href", "") or xml_text(child)
                    break

        information.append(
            {
                "url": link,
                "title": xml_text(xml_child(entry, ["title"])),
                "date_published": xml_text(xml_child(entry, ["pubDate", "published", "updated"])),
                "description": xml_text(xml_child(entry, ["description", "summary"])),
            }
        )

    return information


class LinkExtractor(HTMLParser):
    def __init__(self, base_url: str) -> None:
        super().__init__()
        self.base_url = base_url
        self.links: list[dict[str, str]] = []
        self._href: str | None = None
        self._text: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        attr = {key.lower(): (value or "") for key, value in attrs}
        if tag.lower() == "a" and attr.get("href"):
            self._href = urllib.parse.urljoin(
                self.base_url, html.unescape(attr["href"]))
            self._text = []

    def handle_data(self, data: str) -> None:
        if self._href:
            self._text.append(data)

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "a" and self._href:
            title = re.sub(r"\s+", " ", " ".join(self._text)).strip()
            if title:
                self.links.append({"title": title, "url": self._href})
            self._href = None
            self._text = []


def parse_website_information(source: dict[str, Any], processed: dict[str, Any]) -> list[dict[str, Any]]:
    raw_data = processed.get("data")
    if not isinstance(raw_data, str):
        return []

    source_config = source.get("config", {})
    if not isinstance(source_config, dict):
        source_config = {}

    base_url = str(source.get("url") or "")
    parser = LinkExtractor(base_url)
    parser.feed(raw_data)

    include = source_config.get("include_url_regex")
    exclude = source_config.get("exclude_url_regex")
    max_items = int(source_config.get("max_items", 25))
    information: list[dict[str, Any]] = []
    seen: set[str] = set()
    for link in parser.links:
        url = link["url"].split("#", 1)[0]
        title = link["title"]
        if url in seen or url == base_url:
            continue
        if include and not re.search(str(include), url):
            continue
        if exclude and re.search(str(exclude), url):
            continue
        if len(title) < 8:
            continue

        seen.add(url)
        information.append(
            {
                "url": url,
                "title": title,
                "date_published": "",
                "description": "",
            }
        )
        if len(information) >= max_items:
            break

    return information


def text_without_html(value: Any) -> str:
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", html.unescape(str(value or "")))).strip()


def parse_api_information(source: dict[str, Any], processed: dict[str, Any]) -> list[dict[str, Any]]:
    data = processed.get("data")
    if not isinstance(data, dict):
        return []

    disruptions = data.get("disruptions", [])
    if not isinstance(disruptions, list):
        return []

    lines_by_id = {
        line.get("id"): f"{line.get('mode', '')} {line.get('shortName') or line.get('name') or ''}".strip()
        for line in data.get("lines", [])
        if isinstance(line, dict)
    }

    information: list[dict[str, Any]] = []
    for disruption in disruptions:
        if not isinstance(disruption, dict):
            continue

        impacted_lines = sorted(
            {
                lines_by_id.get(section.get("lineId"), section.get("lineId"))
                for section in disruption.get("impactedSections", [])
                if isinstance(section, dict) and section.get("lineId")
            }
        )
        details = [
            ", ".join(line for line in impacted_lines if line),
            disruption.get("severity"),
            disruption.get("cause"),
            text_without_html(disruption.get("message")
                              or disruption.get("shortMessage")),
        ]

        information.append(
            {
                "url": source.get("url"),
                "title": text_without_html(disruption.get("title") or disruption.get("shortMessage")),
                "date_published": disruption.get("lastUpdate"),
                "description": " | ".join(str(detail) for detail in details if detail),
            }
        )

    return information


def parse_processed_source(source: dict[str, Any]) -> list[dict[str, Any]]:
    source_type = str(source.get("type", "rss")).lower()
    processed = source.get("processed", {})
    if not isinstance(processed, dict):
        return []
    if source_type in {"rss", "feed", "atom", "xml"}:
        return parse_rss_information(processed)
    if source_type in {"website", "html", "web"}:
        return parse_website_information(source, processed)
    if source_type == "api":
        return parse_api_information(source, processed)
    raise ValueError(
        f"unsupported source type {source_type!r} for {source.get('name')}")


def parse_processed_data(data: dict[str, Any]) -> dict[str, Any]:
    processed_sources = data.get("processed_sources", [])
    if not isinstance(processed_sources, list):
        raise ValueError(
            "expected processed data to contain a 'processed_sources' list")

    empty_information_sources: list[dict[str, Any]] = []
    for source in processed_sources:
        if not isinstance(source, dict) or not source.get("ok"):
            continue

        try:
            source["information"] = parse_processed_source(source)
        except Exception as exc:
            source["information"] = []
            source["parse_error"] = str(exc)

        if not source["information"]:
            empty_source = {
                "theme": source.get("theme"),
                "name": source.get("name"),
                "url": source.get("url"),
                "type": source.get("type", "rss"),
            }
            if source.get("parse_error"):
                empty_source["parse_error"] = source.get("parse_error")
            empty_information_sources.append(empty_source)

    data["empty_information_source_count"] = len(empty_information_sources)
    data["empty_information_sources"] = empty_information_sources
    return data


def printable_processed_sources(data: dict[str, Any]) -> list[dict[str, Any]]:
    processed_sources = data.get("processed_sources", [])
    if not isinstance(processed_sources, list):
        raise ValueError(
            "expected processed data to contain a 'processed_sources' list")

    printable_information: list[dict[str, Any]] = []
    information_index = 1
    for source in processed_sources:
        if not isinstance(source, dict):
            continue

        source_information = source.get("information", [])
        if not isinstance(source_information, list):
            continue

        for information in source_information:
            if not isinstance(information, dict):
                continue

            printable_item: dict[str, Any] = {
                "index": information_index,
                "source": source.get("name"),
            }
            if information.get("title"):
                printable_item["title"] = information.get("title")
            if information.get("description"):
                printable_item["description"] = information.get("description")

            printable_information.append(printable_item)
            information_index += 1

    return printable_information


def print_information_counts_by_source(data: dict[str, Any]) -> None:
    processed_sources = data.get("processed_sources", [])
    if not isinstance(processed_sources, list):
        raise ValueError(
            "expected processed data to contain a 'processed_sources' list")

    for source in processed_sources:
        if not isinstance(source, dict):
            continue

        information = source.get("information", [])
        information_count = len(information) if isinstance(information, list) else 0
        print(f"{source.get('name')}: {information_count} information items")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--sources",
        type=Path,
        default=DEFAULT_SOURCES,
        help=f"path to sources.json (default: {DEFAULT_SOURCES})",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    data = collect_from_sources(args.sources)
    data = process_sources(data)
    data = parse_processed_data(data)
    print_information_counts_by_source(data)
    # print processed sources  to file
    (REPO / "processed_sources.json").write_text(
        json.dumps(printable_processed_sources(data), ensure_ascii=False
                   # , separators=(",", ":")
                   ),
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
