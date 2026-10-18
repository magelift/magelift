#!/usr/bin/env python3
"""Regenerate index.md for every directory in an OKF bundle.

Reads each note's own frontmatter, so it works on a bundle directly and needs
no knowledge of where the bundle came from. Run it after adding notes.

  .agents/knowledge/tools/rebuild-index.py .agents/knowledge
"""

import os
import sys
import urllib.parse
from collections import defaultdict

import yaml

RESERVED = {"index.md", "log.md"}
SKIP_DIRS = {".git", "tools", "node_modules"}
SPEC = "https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md"


def bundle_name(bundle):
    """`<repo>/.agents/knowledge` (or legacy `docs/knowledge`) reads as the repo name."""
    parts = os.path.abspath(bundle).rstrip("/").split(os.sep)
    if len(parts) >= 3 and parts[-2:] == ["docs", "knowledge"]:
        return f"{parts[-3]} knowledge"
    if len(parts) >= 3 and parts[-2:] == [".agents", "knowledge"]:
        return f"{parts[-3]} knowledge"
    return parts[-1] or "knowledge"


def read_meta(path):
    text = open(path, encoding="utf-8", errors="replace").read()
    if not text.startswith("---"):
        return {}, text
    end = text.find("\n---", 3)
    if end < 0:
        return {}, text
    try:
        fm = yaml.safe_load(text[3:end]) or {}
    except yaml.YAMLError:
        fm = {}
    return (fm if isinstance(fm, dict) else {}), text[end + 4 :]


def first_sentence(body):
    for line in body.splitlines():
        line = line.strip()
        if not line or line.startswith(("#", ">", "|", "-", "*", "`")):
            continue
        return line[:200]
    return ""


def collect(bundle):
    by_dir = defaultdict(list)
    by_dir[""]
    for dirpath, dirnames, filenames in os.walk(bundle):
        dirnames[:] = [
            d for d in dirnames if d not in SKIP_DIRS and not d.startswith(".")
        ]
        for fn in sorted(filenames):
            if not fn.endswith(".md") or fn in RESERVED:
                continue
            path = os.path.join(dirpath, fn)
            rel = os.path.relpath(path, bundle)
            fm, body = read_meta(path)
            title = str(fm.get("title") or os.path.splitext(fn)[0])
            desc = str(fm.get("description") or first_sentence(body) or "")
            by_dir[os.path.dirname(rel)].append((title, fn, desc, fm))
    return by_dir


def render_index(bundle, d, by_dir):
    lines = []
    if d == "":
        lines += ["---", 'okf_version: "0.2"', "---", ""]
        lines.append(f"# {bundle_name(bundle)}\n")
        lines.append(
            f"Knowledge bundle in [OKF]({SPEC}) format. Read this index, grep, "
            "then open only the notes you need. Authoring rules: "
            "`CONVENTIONS.md`.\n"
        )
    else:
        lines.append(f"# {d}\n")

    here = sorted(by_dir.get(d, []), key=lambda e: e[0].lower())
    if here:
        lines.append("## Concepts\n")
        for title, fn, desc, fm in here:
            flags = []
            if fm.get("status") == "deprecated":
                flags.append("deprecated")
            if fm.get("status") == "draft":
                flags.append("draft")
            if fm.get("stale_after"):
                flags.append(f"stale after {fm['stale_after']}")
            env = fm.get("env")
            if env and env != "all" and env != ["all"]:
                ids = env if isinstance(env, list) else [env]
                flags.append("env " + ",".join(str(i) for i in ids))
            suffix = f" _({', '.join(flags)})_" if flags else ""
            entry = f"* [{title}]({urllib.parse.quote(fn)}){suffix}"
            lines.append(entry + (f" - {desc}" if desc else ""))
        lines.append("")

    nested = sorted(p for p in by_dir if p and os.path.dirname(p) == d and p != d)
    if nested:
        lines.append("## Sections\n")
        for s in nested:
            n = sum(
                len(v) for k, v in by_dir.items() if k == s or k.startswith(s + "/")
            )
            name = os.path.basename(s)
            lines.append(f"* [{name}]({urllib.parse.quote(name)}/index.md) - {n} notes")
        lines.append("")

    return "\n".join(lines)


def main(bundles):
    if not bundles:
        print(__doc__)
        return 2
    for bundle in bundles:
        if not os.path.isdir(bundle):
            print(f"{bundle}: not a directory")
            continue
        by_dir = collect(bundle)
        written = 0
        for d in sorted(by_dir):
            path = os.path.join(bundle, d, "index.md")
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(render_index(bundle, d, by_dir))
            written += 1
        total = sum(len(v) for v in by_dir.values())
        print(f"{bundle}: {written} index.md for {total} notes")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
