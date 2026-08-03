#!/usr/bin/env python3
"""Emit root sitemap.xml + sitemap.txt from priority URLs + built docs tree."""
from __future__ import annotations

import sys
from pathlib import Path

PRIORITY = [
    "https://magelift.dev/",
    "https://magelift.dev/pricing.md",
    "https://magelift.dev/llms.txt",
    "https://magelift.dev/llms-full.txt",
    "https://magelift.dev/docs/",
    "https://magelift.dev/docs/faq/",
    "https://magelift.dev/docs/install/",
    "https://magelift.dev/docs/getting-started/",
    "https://magelift.dev/docs/compare-paas/",
    "https://magelift.dev/docs/weekend-migrate/",
    "https://magelift.dev/docs/capability-matrix/",
    "https://magelift.dev/docs/local-vs-cloud/",
    "https://magelift.dev/docs/migrating-from-paas/",
    "https://magelift.dev/docs/architecture/",
    "https://magelift.dev/docs/configuration/",
    "https://magelift.dev/docs/cli-reference/",
    "https://magelift.dev/docs/operations/",
    "https://magelift.dev/docs/bootstrap/",
]


def docs_urls(docs_root: Path) -> list[str]:
    urls: list[str] = []
    if not docs_root.is_dir():
        return urls
    for index in sorted(docs_root.rglob("index.html")):
        rel = index.parent.relative_to(docs_root).as_posix()
        if rel in {".", "assets"} or rel.startswith("assets/") or "/assets/" in f"/{rel}/":
            continue
        # Skip deep evidence/source noise from root SEO sitemap (still on site)
        if rel.startswith("evidence/") or rel.startswith("sources/") or rel.startswith("adr/"):
            continue
        if rel == ".":
            urls.append("https://magelift.dev/docs/")
        else:
            urls.append(f"https://magelift.dev/docs/{rel}/")
    return urls


def main() -> int:
    site = Path(__file__).resolve().parents[1]
    public = site / "public"
    docs_root = public / "docs"
    urls: list[str] = []
    seen: set[str] = set()
    for u in PRIORITY + docs_urls(docs_root):
        if u not in seen:
            seen.add(u)
            urls.append(u)

    txt = "\n".join(urls) + "\n"
    (public / "sitemap.txt").write_text(txt)
    lines = [
        '<?xml version="1.0" encoding="UTF-8"?>',
        '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ]
    for u in urls:
        lines.append(f"  <url><loc>{u}</loc></url>")
    lines.append("</urlset>")
    (public / "sitemap.xml").write_text("\n".join(lines) + "\n")
    print(f"sitemap: {len(urls)} urls", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
