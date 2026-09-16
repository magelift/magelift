---
status: done
slug: site-docs-hygiene
spec: spec.md
---

# Plan: site and docs hygiene

## Files that change

**Edit (2 source edits + 1 regeneration):**

- `internal/config/schema.go` — header insertion in `ReferenceMarkdown()` (lines 32-34). Split the line-34 `WriteString` so the title `"# Configuration reference\n\n"` is followed by `"This page is generated from the MageLift configuration schema. Do not edit it by hand.\n\n"`, then the existing `` `magelift.yaml` `` prose. This is the only code edit for fix 1.
- `docs/configuration.md` — regenerated output only, via `make generate`. Never hand-edit.
- `.gitignore` (root) — append 2 lines to the website section (lines 13-17), directly after line 17 (`/website/public/docs/`):
  - `/website/public/sitemap.txt`
  - `/website/public/sitemap.xml`

**Untrack (working tree kept):**

- `website/public/sitemap.txt` + `website/public/sitemap.xml` — currently tracked (confirmed: `git ls-files website/public/sitemap.txt website/public/sitemap.xml` returns both). Untrack via `git rm --cached website/public/sitemap.txt website/public/sitemap.xml` (keeps working-tree files).

**Verify-only (zero edits):**

- `docs/aws-acceptance.md:140` — `[acceptance command dependencies](acceptance-dependencies.md)` in Prerequisites section (confirmed verbatim).
- `docs/gcp-acceptance.md:359` — `[acceptance command dependencies](acceptance-dependencies.md)` in Credentials section (confirmed verbatim; line wraps, link text contiguous).
- `mkdocs.yml` — absence confirmed (`grep 'acceptance-dependencies' mkdocs.yml` exits 1). No nav entry added.

**NOT touched:**

- `cmd/genconfig/main.go` — thin writer (`{path: "docs/configuration.md", data: config.ReferenceMarkdown()}` at line 24) owns no content; byte-exact `--check` wiring stays as-is.
- `website/scripts/generate-sitemap.py` — write lines 60 and 68 unchanged; no sitemap content change.
- `mkdocs.yml` nav, `docs/acceptance-dependencies.md`, and all human prose — orphan resolution is verification-only.
- Humanizer / remove-ai-marks — skipped; all changes are mechanical (generated header mirror, ignore paths, verification-only).

**Wiring reference (read at research time):**

- `internal/config/schema_test.go:71` `TestReferenceMarkdownIsDeterministic` asserts directly on `ReferenceMarkdown()` output.
- `internal/config/generate.go:3` (`//go:generate go run ../../cmd/genconfig -root ../..`) + `Makefile:266` `generate` regenerate the page; `Makefile:269` `generate-check` verifies it; `Makefile:251` `docs` (`"$(scripts/docs-venv.sh)/mkdocs" build --strict`) is the docs gate.

## Order of work

### 1. Generated header via `ReferenceMarkdown()` + stability

- [x] 1.1 Edit `internal/config/schema.go:34` to emit title, blank, `This page is generated from the MageLift configuration schema. Do not edit it by hand.`, blank, then existing prose; run `make generate` once — verify: `grep -F 'This page is generated from the MageLift configuration schema. Do not edit it by hand.' docs/configuration.md` exits 0 and `sed -n '1,5p' docs/configuration.md` shows `# Configuration reference`, blank, header, blank, `` `magelift.yaml` uses schema version 1. Unknown fields are rejected. ``.
- [x] 1.2 Run `make generate` a second time, then `make generate-check`, then confirm no drift in generated outputs — verify: `make generate-check` exits 0 and `git diff --exit-code -- docs/configuration.md schema/magelift.schema.json` exits 0.

### 2. Sitemap untrack + rebuild-clean check

- [x] 2.1 Append `/website/public/sitemap.txt` and `/website/public/sitemap.xml` after the `/website/public/docs/` line in root `.gitignore`, then untrack with `git rm --cached website/public/sitemap.txt website/public/sitemap.xml` (keeps working-tree files; never bare `git rm`) — verify: `git ls-files website/public/sitemap.txt website/public/sitemap.xml` returns empty and `grep -F '/website/public/sitemap.txt' .gitignore` and `grep -F '/website/public/sitemap.xml' .gitignore` both exit 0.
- [x] 2.2 Run `website/scripts/build-site.sh` (regenerates sitemaps via `generate-sitemap.py`), then confirm both files exist on disk with no tracked churn — verify: `test -f website/public/sitemap.txt && test -f website/public/sitemap.xml` exits 0 and `git status --porcelain -- website/public/sitemap.txt website/public/sitemap.xml` outputs nothing. Subset fallback only if the build fails solely at `npm ci`/`npm install` with a registry network error: run the MkDocs build + `python3 website/scripts/generate-sitemap.py` lines directly, then re-check existence + clean status.

### 3. Orphan link verification (zero content change)

- [x] 3.1 Confirm both natural parents link to the orphan page in context (re-grep at apply time; links verified during research at `docs/aws-acceptance.md:140` and `docs/gcp-acceptance.md:359`) — verify: `grep -rn 'acceptance-dependencies' docs/ mkdocs.yml website/ --include='*.md' --include='*.yml' --include='*.astro' --include='*.js'` output includes `docs/aws-acceptance.md` containing `[acceptance command dependencies](acceptance-dependencies.md)` and `docs/gcp-acceptance.md` containing `[acceptance command dependencies](acceptance-dependencies.md)`.
- [x] 3.2 Confirm link is the single resolution with no nav entry — verify: `grep 'acceptance-dependencies' mkdocs.yml` exits 1.

### 4. Docs gate

- [x] 4.1 Run the strict MkDocs build after all three fixes — verify: `make docs` exits 0 with no strict-mode warnings or errors.

## Risks

- **Hand-editing `docs/configuration.md` directly** — fails `make generate-check` (`cmd/genconfig --check` byte-compares against `ReferenceMarkdown()`). This plan forbids it: edit `internal/config/schema.go`, then `make generate`.
- **Sitemap `git rm` without `--cached` deleting working files** — would force a rebuild to recover. Pinned exact command: `git rm --cached website/public/sitemap.txt website/public/sitemap.xml`.
- **`website/scripts/build-site.sh` npm/network flakiness vs the sitemap check** — `build-site.sh` runs `npm ci`/`npm run build` after regenerating sitemaps; a network failure there is unrelated to this change. Subset fallback only if npm fails: run the MkDocs build + `python3 website/scripts/generate-sitemap.py` lines directly, then re-check existence + clean status.
- **Parent-link drift since research** — links verified during research; re-grep at apply time (step 3.1) before declaring the orphan resolved.

## Proof

```bash
# Fix 1: header present with exact text, file head correct
sed -n '1,5p' docs/configuration.md
grep -F 'This page is generated from the MageLift configuration schema. Do not edit it by hand.' docs/configuration.md

# Fix 1: generate-check stable across runs
make generate
make generate
make generate-check
git diff --exit-code -- docs/configuration.md schema/magelift.schema.json

# Fix 2: files untracked and ignored
git ls-files website/public/sitemap.txt website/public/sitemap.xml
grep -F '/website/public/sitemap.txt' .gitignore
grep -F '/website/public/sitemap.xml' .gitignore

# Fix 2: site build regenerates without dirtying the tree
website/scripts/build-site.sh
test -f website/public/sitemap.txt && test -f website/public/sitemap.xml
git status --porcelain -- website/public/sitemap.txt website/public/sitemap.xml

# Fix 3: links present from both natural parents, link is the single resolution
grep -rn 'acceptance-dependencies' docs/ mkdocs.yml website/ --include='*.md' --include='*.yml' --include='*.astro' --include='*.js'
grep 'acceptance-dependencies' mkdocs.yml; test $? -eq 1

# Gate: docs build stays green
make docs
```
