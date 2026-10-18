#!/usr/bin/env python3
"""Check OKF v0.2 conformance for one or more bundles.

A bundle conforms when every non-reserved .md file has parseable YAML
frontmatter carrying a non-empty `type`. Everything else the spec calls
optional is reported as a warning, never a failure — consumers MUST NOT
reject a bundle over missing optional fields or broken links.

  .agents/knowledge/tools/validate-okf.py .agents/knowledge
"""

import os
import re
import sys
import urllib.parse

import yaml

RESERVED = {"index.md", "log.md"}
LIFECYCLE = {"draft", "stable", "deprecated"}
ENV_IDS = {"wsl2", "macos", "devbox", "all"}
HOST_HINT = re.compile(
    r"\b(wsl2?|macos|macbook|darwin|devbox|hetzner|/home/dev|/Users/)\b", re.I
)
MDLINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def _env_ids(value):
    if value is None or value == "all":
        return []
    if isinstance(value, list):
        return [str(i) for i in value]
    return [str(value)]


def check(bundle):
    errors, warnings, notes = [], [], 0
    titles = {}
    for dirpath, dirnames, filenames in os.walk(bundle):
        # Dot-directories are staging and tooling state, not part of the bundle.
        dirnames[:] = [d for d in dirnames if not d.startswith(".") and d != "tools"]
        for fn in sorted(filenames):
            if not fn.endswith(".md"):
                continue
            path = os.path.join(dirpath, fn)
            rel = os.path.relpath(path, bundle)
            text = open(path, encoding="utf-8", errors="replace").read()

            if fn in RESERVED:
                # index.md may carry only okf_version, and only at bundle root.
                if text.startswith("---"):
                    end = text.find("\n---", 3)
                    fm = yaml.safe_load(text[3:end]) or {} if end > 0 else {}
                    extra = set(fm) - {"okf_version"}
                    if extra:
                        warnings.append(
                            f"{rel}: reserved file has extra frontmatter {sorted(extra)}"
                        )
                    if "okf_version" in fm and os.path.dirname(rel):
                        errors.append(
                            f"{rel}: okf_version is only allowed in the bundle-root index.md"
                        )
                continue

            notes += 1
            if not text.startswith("---"):
                errors.append(f"{rel}: no frontmatter")
                continue
            end = text.find("\n---", 3)
            if end < 0:
                errors.append(f"{rel}: unterminated frontmatter")
                continue
            try:
                fm = yaml.safe_load(text[3:end])
            except yaml.YAMLError as e:
                errors.append(
                    f"{rel}: unparseable frontmatter ({str(e).splitlines()[0]})"
                )
                continue
            if not isinstance(fm, dict):
                errors.append(f"{rel}: frontmatter is not a mapping")
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
                warnings.append(f"{rel}: no description (weakens index.md entries)")

            title_key = str(fm.get("title") or "").strip().lower()
            if title_key:
                titles.setdefault(title_key, []).append(rel)

            env_ids = _env_ids(fm.get("env"))
            bad = [i for i in env_ids if i not in ENV_IDS]
            if bad:
                warnings.append(f"{rel}: env has unknown id {bad}")
            rel_dir = os.path.dirname(rel).replace("\\", "/")
            if rel_dir == "notes" or rel_dir.endswith("/notes"):
                warnings.append(
                    f"{rel}: notes/ is a junk drawer — prefer lessons/reference/sources"
                )
            hinted = f"{rel} {fm.get('title') or ''} {fm.get('description') or ''}"
            tags = fm.get("tags") or []
            if isinstance(tags, str):
                tags = [tags]
            documents_env = (
                "env" in {str(t).lower() for t in tags}
                or str(fm.get("title") or "").lower() == "host ids"
            )
            if not env_ids and HOST_HINT.search(hinted) and not documents_env:
                warnings.append(f"{rel}: host-specific title/description but no env:")

            body = text[end + 4 :]
            for target in MDLINK.findall(body):
                if target.startswith(("http://", "https://", "#", "mailto:")):
                    continue
                p = urllib.parse.unquote(target.split("#")[0])
                resolved = (
                    os.path.join(bundle, p.lstrip("/"))
                    if p.startswith("/")
                    else os.path.join(dirpath, p)
                )
                if not os.path.exists(resolved):
                    warnings.append(f"{rel}: broken link -> {target}")

    for title, rels in titles.items():
        if len(rels) > 1:
            warnings.append(f"duplicate title {title!r}: {', '.join(rels)}")

    return notes, errors, warnings


def main(bundles):
    if not bundles:
        print(__doc__)
        return 2
    total_err = 0
    for b in bundles:
        if not os.path.isdir(b):
            print(f"{b}: not a directory")
            continue
        notes, errors, warnings = check(b)
        root_index = os.path.join(b, "index.md")
        status = "CONFORMS" if not errors else f"{len(errors)} ERRORS"
        print(
            f"\n{b}\n  notes: {notes:4d}   root index.md: "
            f"{'yes' if os.path.exists(root_index) else 'MISSING'}   -> {status}"
        )
        for e in errors[:20]:
            print(f"    ERROR  {e}")
        by_kind = {}
        for w in warnings:
            kind = w.split(": ", 1)[1].split(" ->")[0].split(" (")[0]
            by_kind[kind] = by_kind.get(kind, 0) + 1
        for kind, n in sorted(by_kind.items(), key=lambda x: -x[1]):
            print(f"    warn   {n:4d}x  {kind}")
        total_err += len(errors)
    print(f"\ntotal errors: {total_err}")
    return 1 if total_err else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
