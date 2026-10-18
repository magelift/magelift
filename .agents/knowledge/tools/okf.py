#!/usr/bin/env python3
"""Rewrite index.md files and check this OKF bundle.

.agents/knowledge/tools/okf.py .agents/knowledge
"""

import os
import re
import sys
import urllib.parse
from collections import defaultdict

import yaml

RESERVED = {"index.md", "log.md"}
SKIP_DIRS = {".git", "tools", "node_modules"}
LIFECYCLE = {"draft", "stable", "deprecated"}
SPEC = "https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md"
LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def bundle_name(bundle):
    parts = os.path.abspath(bundle).rstrip("/").split(os.sep)
    if len(parts) >= 3 and parts[-2:] == [".agents", "knowledge"]:
        return f"{parts[-3]} knowledge"
    return parts[-1] or "knowledge"


def read_note(path):
    text = open(path, encoding="utf-8", errors="replace").read()
    if not text.startswith("---"):
        return {"_error": "no frontmatter"}, ""
    end = text.find("\n---", 3)
    if end < 0:
        return {"_error": "unterminated frontmatter"}, ""
    try:
        fm = yaml.safe_load(text[3:end]) or {}
    except yaml.YAMLError as exc:
        return {"_error": f"unparseable frontmatter ({str(exc).splitlines()[0]})"}, ""
    if not isinstance(fm, dict):
        return {"_error": "frontmatter is not a mapping"}, ""
    return fm, text[end + 4 :]


def first_sentence(body):
    for line in body.splitlines():
        line = line.strip()
        if line and not line.startswith(("#", ">", "|", "-", "*", "`")):
            return line[:200]
    return ""


def collect(bundle):
    found = defaultdict(list)
    found[""]
    for dirpath, dirnames, filenames in os.walk(bundle):
        dirnames[:] = [
            d for d in dirnames if d not in SKIP_DIRS and not d.startswith(".")
        ]
        for fn in sorted(filenames):
            if not fn.endswith(".md") or fn in RESERVED:
                continue
            path = os.path.join(dirpath, fn)
            rel = os.path.relpath(path, bundle)
            fm, body = read_note(path)
            title = str(fm.get("title") or os.path.splitext(fn)[0])
            desc = str(fm.get("description") or first_sentence(body) or "")
            found[os.path.dirname(rel)].append(
                (title, fn, desc, fm, rel, dirpath, body)
            )
    return found


def render_index(bundle, directory, found):
    lines = []
    if directory == "":
        lines += ["---", 'okf_version: "0.2"', "---", ""]
        lines.append(f"# {bundle_name(bundle)}\n")
        lines.append(
            f"Knowledge bundle in [OKF]({SPEC}) format. Read this index, grep, "
            "then open only the notes you need. Authoring rules: `conventions.md`.\n"
        )
    else:
        lines.append(f"# {directory}\n")

    here = sorted(found.get(directory, []), key=lambda item: item[0].lower())
    if here:
        lines.append("## Concepts\n")
        for title, fn, desc, fm, *_rest in here:
            flags = []
            if fm.get("status") == "deprecated":
                flags.append("deprecated")
            if fm.get("status") == "draft":
                flags.append("draft")
            if fm.get("stale_after"):
                flags.append(f"stale after {fm['stale_after']}")
            suffix = f" _({', '.join(flags)})_" if flags else ""
            line = f"* [{title}]({urllib.parse.quote(fn)}){suffix}"
            lines.append(line + (f" - {desc}" if desc else ""))
        lines.append("")

    nested = sorted(
        path for path in found if path and os.path.dirname(path) == directory
    )
    if nested:
        lines.append("## Sections\n")
        for section in nested:
            count = sum(
                len(items)
                for path, items in found.items()
                if path == section or path.startswith(section + "/")
            )
            name = os.path.basename(section)
            lines.append(
                f"* [{name}]({urllib.parse.quote(name)}/index.md) - {count} notes"
            )
        lines.append("")
    return "\n".join(lines)


def link_target(bundle, dirpath, raw):
    path = urllib.parse.unquote(raw.split("#")[0])
    if path.startswith("/"):
        return os.path.join(bundle, path.lstrip("/"))
    return os.path.join(dirpath, path)


def check(bundle, found):
    errors, warnings, titles = [], [], {}
    count = 0
    for items in found.values():
        for _title, _fn, _desc, fm, rel, dirpath, body in items:
            count += 1
            if fm.get("_error"):
                errors.append(f"{rel}: {fm['_error']}")
                continue
            if not str(fm.get("type") or "").strip():
                errors.append(f"{rel}: missing or empty required field `type`")
            status = fm.get("status")
            if status is not None and status not in LIFECYCLE:
                warnings.append(
                    f"{rel}: status={status!r} outside draft|stable|deprecated"
                )
            if not fm.get("title"):
                warnings.append(f"{rel}: no title")
            if not fm.get("description"):
                warnings.append(f"{rel}: no description")
            key = str(fm.get("title") or "").strip().lower()
            if key:
                titles.setdefault(key, []).append(rel)
            if os.path.basename(os.path.dirname(rel)) == "notes":
                warnings.append(f"{rel}: notes/ is a junk drawer")
            for raw in LINK.findall(body):
                if raw.startswith(("http://", "https://", "#", "mailto:")):
                    continue
                if not os.path.exists(link_target(bundle, dirpath, raw)):
                    warnings.append(f"{rel}: broken link -> {raw}")
    for title, rels in titles.items():
        if len(rels) > 1:
            warnings.append(f"duplicate title {title!r}: {', '.join(rels)}")
    return count, errors, warnings


def main(bundles):
    if not bundles:
        print(__doc__)
        return 2
    total_err = 0
    for bundle in bundles:
        if not os.path.isdir(bundle):
            print(f"{bundle}: not a directory")
            total_err += 1
            continue
        found = collect(bundle)
        written = 0
        for directory in sorted(found):
            path = os.path.join(bundle, directory, "index.md")
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(render_index(bundle, directory, found))
            written += 1
        count, errors, warnings = check(bundle, found)
        status = "CONFORMS" if not errors else f"{len(errors)} ERRORS"
        print(f"{bundle}: {written} index.md for {count} notes -> {status}")
        for error in errors[:20]:
            print(f"  ERROR  {error}")
        by_kind = {}
        for warning in warnings:
            kind = warning.split(": ", 1)[-1].split(" ->")[0]
            by_kind[kind] = by_kind.get(kind, 0) + 1
        for kind, n in sorted(by_kind.items(), key=lambda item: -item[1]):
            print(f"  warn   {n:4d}x  {kind}")
        total_err += len(errors)
    return 1 if total_err else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
