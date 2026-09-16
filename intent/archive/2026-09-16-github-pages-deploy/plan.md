---
status: done
slug: github-pages-deploy
spec: spec.md
---

# Plan: GitHub Pages single deploy story

## Files that change

### Pre-deletion verification targets (read-only gates; no ops yet)

- `.cloudflare/cache/cloudflare-account.json` — Delete (via `git rm`), but shape-check first: 116B, sole `git ls-files .cloudflare/` hit, shape `['account']` / `{'account': 'dict'}`; never `cat`.
- `.github/workflows/docs.yml` — Delete (via `git rm`), but branch-protection check first: 42 lines, `name: Docs site` line 1, `mkdocs-site` artifact line 40, no `pull_request` trigger.

### Deletions

- `website/public/_headers` — Delete (`git rm`): 18 lines of Cloudflare-only path-block syntax (`/llms.txt`, `/llms-full.txt`, `/pricing.md`, `/robots.txt`, `/sitemap.xml`); Pages ignores it.
- `website/public/_redirects` — Delete (`git rm`): 4 lines Netlify/Cloudflare-only syntax; line 1 is an invalid `#!/usr/bin/env bash` — do not fix, delete.
- `.cloudflare/cache/cloudflare-account.json` — Delete (`git rm`, index + tree) after the keys/types-only shape check.
- `.github/workflows/docs.yml` — Delete (`git rm`) after the branch-protection check; stale "Pages unavailable" header comment lines 3-5.

### Edits

- `.gitignore` — Edit (one line): append `.cloudflare/` next to the existing `.wrangler/` entry at line 46.
- `contrib/skills/magelift-site/SKILL.md` — Edit (`## Deploy` lines 35-43 only): replace wrangler block with the verbatim Pages wording in step 3.1; leave `## Stack`, `## Copy rules`, `## Use this skill when`, `## Leave behind` untouched.

### Verify-only (no change expected; humanizer + remove-ai-marks if drift forces an edit)

- `website/README.md` — Verify-only: line 28 already states Public site workflow deploys `dist/` to GitHub Pages.
- `docs/publishing.md` — Verify-only: Build row line 25 and launch checklist line 45 already state GitHub Pages Done.
- `website/public/CNAME` — Verify-only / Keep byte-identical: 13B, `magelift.dev\n`.
- `.github/workflows/site.yml` — Verify-only / Keep: sole deploy owner; deploy job gated line 54 (`github.event_name == 'push' && github.ref == 'refs/heads/main'`), `actions/deploy-pages` line 65; PRs build only via `pull_request` trigger lines 14-20.
- `website/scripts/build-site.sh` — Keep: no `_headers`/`_redirects` refs; line 14 runs `mkdocs build --strict`; Astro copies `public/` verbatim so deletion propagates to `dist/` automatically.
- `website/public/getting-started/index.html` — Verify-only: line 6 meta-refresh `url=/docs/getting-started/` + canonical survive the `_redirects` deletion.
- `website/astro.config.mjs` — Verify-only: line 4 `site: 'https://magelift.dev'`; must not diff.
- `mkdocs.yml` — Verify-only: line 3 `site_url: https://magelift.dev/docs/`; must not diff.
- `.github/workflows/ci.yml` — Verify-only (safety case): `docs` job lines 192-207 owns PR strict builds via path filter; repo-side gate is the `CI passed` aggregation, not `Docs site`.

## Order of work

### 1.x Live-host + shape checks (all pass before any deletion)

- [x] 1.1 Run the primary live-host check: expect apex served by GitHub Pages — verify: `curl -sI --max-time 10 https://magelift.dev/ | grep -i server` outputs exactly `server: GitHub.com`, and `curl -sI --max-time 10 https://magelift.dev/` contains `x-github-request-id:` with no `cf-ray:`, and `dig +short www.magelift.dev CNAME` returns `magelift.github.io.`, and `dig +short magelift.dev A` returns addresses in `185.199.108.153` / `185.199.109.153` / `185.199.110.153` / `185.199.111.153`.
- [x] 1.2 If 1.1 fails with curl exit 6 or 28 only, record `live-check: network-unavailable` with the exit code and take the no-network fallback instead; deletions SHALL NOT proceed until 1.1 or 1.2 passes — verify: `gh api repos/magelift/magelift/pages --jq '.cname'` returns `magelift.dev`.
- [x] 1.3 Run the `.cloudflare/` pre-delete shape check (key names + type names only; never print values, never `cat` the file) — verify: `python3 -c "import json;d=json.load(open('.cloudflare/cache/cloudflare-account.json'));print(sorted(d.keys()));print({k:type(v).__name__ for k,v in d.items()})"` shows `['account']` and `{'account': 'dict'}` (account metadata, no tokens).
- [x] 1.4 Check branch protection for a required `Docs site` context before deleting `docs.yml` (repo-side gate is the `CI passed` aggregation in `ci.yml`, but settings live outside the repo); if the API is unavailable, record a manual branch-protection UI check with the same expected result before merging — verify: `gh api repos/magelift/magelift/branches/main/protection --jq '.required_status_checks.contexts[]'` returns no context equal to `Docs site`.

### 2.x Deletions + ignore

- [x] 2.1 `git rm website/public/_headers website/public/_redirects` (both Cloudflare/Netlify-only; Pages ignores them) — verify: `test ! -e website/public/_headers && test ! -e website/public/_redirects` exits 0, and `git ls-files website/public/ | grep -E '_headers|_redirects'` returns empty.
- [x] 2.2 `git rm .cloudflare/cache/cloudflare-account.json` (only file under `.cloudflare/`), then append `.cloudflare/` to `.gitignore` next to the line-46 `.wrangler/` entry — verify: `git ls-files .cloudflare/` returns empty, and `grep -F '.cloudflare/' .gitignore` exits 0.
- [x] 2.3 `git rm .github/workflows/docs.yml` (redundant: PR strict-build owned by `ci.yml` lines 192-207, `main` coverage owned by `site.yml` via `build-site.sh` line 14, `mkdocs-site` artifact unconsumed). Note: the bare `docs\.yml` pattern substring-matches `mkdocs.yml`, so the ref sweep filters those out — verify: `test ! -e .github/workflows/docs.yml` exits 0, and `grep -rln "docs\.yml" .github/ docs/ website/ contrib/ Makefile scripts/ | grep -v "mkdocs\.yml"` returns empty, and `grep -rln "mkdocs-site" .github/ docs/ website/ contrib/ Makefile scripts/` returns empty, and `grep -rn "Docs site" .github/` returns empty.

### 3.x Skill edit + ref sweeps

- [x] 3.1 Replace lines 35-43 of `contrib/skills/magelift-site/SKILL.md` with this verbatim block (Pages only, no wrangler/Cloudflare path):

```md
## Deploy

Push to `main`. `.github/workflows/site.yml` (Public site) rebuilds Astro +
MkDocs via `website/scripts/build-site.sh` and deploys `website/dist` to
GitHub Pages with `actions/deploy-pages`. PRs build only; only a `main`
push deploys.

Custom domain: `magelift.dev` (via `website/public/CNAME`); `www` redirects
to the apex. Docs live at `/docs/` inside the same deploy — there is no
separate docs host, redirect file, or header file.
```

— verify: `grep -in "wrangler\|cloudflare" contrib/skills/magelift-site/SKILL.md` returns empty, and `grep -F "npx wrangler pages deploy dist --project-name=magelift --branch=main" contrib/skills/magelift-site/SKILL.md` returns empty, and `grep -F ".github/workflows/site.yml" contrib/skills/magelift-site/SKILL.md` exits 0, and `grep -F "actions/deploy-pages" contrib/skills/magelift-site/SKILL.md` exits 0.
- [x] 3.2 Verify `website/README.md` and `docs/publishing.md` agree on Pages (both already correct; edit only on drift, and any edit goes through humanizer then remove-ai-marks before the diff is final) — verify: `grep -F "GitHub Pages" website/README.md` and `grep -F "GitHub Pages" docs/publishing.md` both exit 0, and `grep -in "wrangler\|cloudflare" website/README.md docs/publishing.md` returns empty.
- [x] 3.3 Confirm `CNAME` untouched (apex domain file) — verify: `test "$(cat website/public/CNAME)" = "magelift.dev"` exits 0.
- [x] 3.4 Confirm no URL changes across config and skill — verify: `grep -F "site: 'https://magelift.dev'" website/astro.config.mjs` and `grep -F "site_url: https://magelift.dev/docs/" mkdocs.yml` both exit 0, and `grep -F "https://magelift.dev/" contrib/skills/magelift-site/SKILL.md` exits 0, and `git diff --exit-code -- website/astro.config.mjs mkdocs.yml website/public/CNAME` exits 0.

### 4.x Build + live-coverage evidence

- [x] 4.1 Run the full site build from the repo root (continuity pre-merge signal: the PR build of `site.yml` is build-only — `pull_request` trigger lines 14-20, deploy gated line 54 — so this passing is the merge gate) — verify: `website/scripts/build-site.sh` exits 0, and `test -d website/dist/docs` exits 0, and `test -f website/dist/CNAME` exits 0, and `test "$(cat website/dist/CNAME)" = "magelift.dev"` exits 0, and `ls website/dist/_headers website/dist/_redirects` fails (no such files), and `grep -rn "_headers\|_redirects" website/scripts/build-site.sh website/astro.config.mjs` returns empty.
- [x] 4.2 Offline fallback ONLY if 4.1 fails solely at the `npm ci` / `npm install` step with a registry network error: run the documented MkDocs subset and record the npm failure output verbatim in the change report (a green subset does not excuse an unexamined npm failure) — verify: `"$("scripts/docs-venv.sh")/mkdocs" build --strict --config-file mkdocs.yml --site-dir /tmp/magelift-docs-probe` exits 0.
- [x] 4.3 Record live coverage for every behavior the deleted files claimed: in-tree `/getting-started` meta-refresh survives; live `/docs` 301, `/llms.txt` MIME, and www-to-apex 301 all served by GitHub Pages natively — verify: `grep -F 'url=/docs/getting-started/' website/public/getting-started/index.html` exits 0, and `curl -sI --max-time 10 https://magelift.dev/docs` returns `server: GitHub.com` with `location: https://magelift.dev/docs/`, and `curl -sI --max-time 10 https://magelift.dev/llms.txt` contains `server: GitHub.com` and `content-type: text/plain; charset=utf-8`, and `curl -sI --max-time 10 https://www.magelift.dev/ | grep -i -E "^(server|location)"` shows `server: GitHub.com` and `location: https://magelift.dev/`.
- [ ] 4.4 Post-merge observation (owner: merger): watch the next `main` Pages deploy, then re-run the apex fingerprint; if the deploy fails, continuity regressed and takes priority over everything else in this spec — verify: `gh run list --workflow=site.yml --branch main` shows the `main` deploy green in the `github-pages` environment, then `curl -sI --max-time 10 https://magelift.dev/ | grep -i server` outputs exactly `server: GitHub.com`.

## Risks

- Deploy-continuity break: the next `main` push must still ship the site; deleting the wrong workflow or breaking the build silences deploys. Check: pre-merge PR build of `site.yml` (build-only on PRs) green per step 4.1, plus post-merge observation per step 4.4; no `site.yml` or `build-site.sh` edits in this change.
- Branch-protection blind spot: required checks live in GitHub settings, not the repo; if an admin ever added `Docs site` as required, deletion breaks the merge queue. Check: step 1.4 `gh api .../branches/main/protection` (or admin UI check) confirms no `Docs site` context before merging; owner for removal is the repo admin.
- Secret-value leak in evidence: `.cloudflare/cache/cloudflare-account.json` must be confirmed metadata-only without its values ever entering logs, evidence, or the diff. Check: step 1.3 keys/types-only query (`['account']` / `{'account': 'dict'}`); never `cat`; grep staged evidence for IDs/tokens before commit.
- Acceptance-helper false positives in greps: bare `cloudflare`/`wrangler` greps over the repo hit out-of-scope acceptance DNS tooling (`scripts/cutover-dns-cloudflare.sh`, `scripts/acceptance/*`, `tests/acceptance/*`, `docs/evidence/*`, ADRs, `Makefile` harness line) — plus bare `docs\.yml` substring-matches every `mkdocs.yml` reference. Check: scope verify greps to `.github/ docs/ website/ contrib/` plus `Makefile`/`scripts/` only for `_headers`/`_redirects`/`docs.yml` patterns; filter `mkdocs.yml` substring hits per step 2.3.
- Npm-offline build fallback masking real failure: the MkDocs-only subset passing while the real Astro build is broken would ship a false green. Check: step 4.2 allowed only when `build-site.sh` fails solely at `npm ci`/`npm install` with a registry network error; npm failure output recorded verbatim; any other failure mode fails the change.

## Proof

```sh
# 1.x host fingerprints (primary; or fallback if curl exit 6/28)
curl -sI --max-time 10 https://magelift.dev/ | grep -i server
# expect: server: GitHub.com
curl -sI --max-time 10 https://magelift.dev/ | grep -i -E '^(server|x-github-request-id|cf-ray)'
# expect: server GitHub.com + x-github-request-id, no cf-ray
dig +short www.magelift.dev CNAME
# expect: magelift.github.io.
dig +short magelift.dev A
# expect: 185.199.108.153 / .109. / .110. / .111.
# fallback only: gh api repos/magelift/magelift/pages --jq '.cname'  # expect: magelift.dev

# 1.x shape check (types only, before rm) + branch protection (before rm)
python3 -c "import json;d=json.load(open('.cloudflare/cache/cloudflare-account.json'));print(sorted(d.keys()));print({k:type(v).__name__ for k,v in d.items()})"
# expect: ['account'] / {'account': 'dict'}
gh api repos/magelift/magelift/branches/main/protection --jq '.required_status_checks.contexts[]'
# expect: no `Docs site` line

# 2.x deletion empties
test ! -e website/public/_headers && test ! -e website/public/_redirects && echo DEL-HEADERS-OK
git ls-files website/public/ | grep -E '_headers|_redirects'; echo "headers-refs-exit=$? (expect 1)"
git ls-files .cloudflare/; echo "cf-tracked-exit=$? (expect 1 = empty)"
grep -F '.cloudflare/' .gitignore && echo IGNORE-OK
test ! -e .github/workflows/docs.yml && echo DEL-DOCSYML-OK
grep -rn "Docs site" .github/; echo "docs-site-exit=$? (expect 1)"
grep -rln "docs\.yml" .github/ docs/ website/ contrib/ Makefile scripts/ | grep -v "mkdocs\.yml"; echo "dangling-exit=$? (expect 1)"
grep -rln "mkdocs-site" .github/ docs/ website/ contrib/ Makefile scripts/; echo "artifact-exit=$? (expect 1)"

# 3.x skill greps + CNAME bytes + URL pins
grep -in "wrangler\|cloudflare" contrib/skills/magelift-site/SKILL.md; echo "skill-residue-exit=$? (expect 1)"
grep -F "npx wrangler pages deploy dist --project-name=magelift --branch=main" contrib/skills/magelift-site/SKILL.md; echo "exact-line-exit=$? (expect 1)"
grep -F ".github/workflows/site.yml" contrib/skills/magelift-site/SKILL.md && grep -F "actions/deploy-pages" contrib/skills/magelift-site/SKILL.md && echo SKILL-PAGES-OK
grep -F "GitHub Pages" website/README.md && grep -F "GitHub Pages" docs/publishing.md && echo README-PUB-OK
grep -in "wrangler\|cloudflare" website/README.md docs/publishing.md; echo "human-residue-exit=$? (expect 1)"
test "$(cat website/public/CNAME)" = "magelift.dev" && echo CNAME-OK
grep -F "site: 'https://magelift.dev'" website/astro.config.mjs && grep -F "site_url: https://magelift.dev/docs/" mkdocs.yml && grep -F "https://magelift.dev/" contrib/skills/magelift-site/SKILL.md && echo URLS-OK
git diff --exit-code -- website/astro.config.mjs mkdocs.yml website/public/CNAME && echo URLS-UNDIFFED-OK

# 4.x build green + dist residue + live redirect/MIME lines
website/scripts/build-site.sh && echo BUILD-OK
test -d website/dist/docs && test -f website/dist/CNAME && test "$(cat website/dist/CNAME)" = "magelift.dev" && echo DIST-OK
ls website/dist/_headers website/dist/_redirects; echo "dist-residue-exit=$? (expect nonzero)"
grep -rn "_headers\|_redirects" website/scripts/build-site.sh website/astro.config.mjs; echo "build-refs-exit=$? (expect 1)"
grep -F 'url=/docs/getting-started/' website/public/getting-started/index.html && echo META-REFRESH-OK
curl -sI --max-time 10 https://magelift.dev/docs | grep -i -E '^(server|location)'
# expect: server: GitHub.com + location: https://magelift.dev/docs/
curl -sI --max-time 10 https://magelift.dev/llms.txt | grep -i -E '^(server|content-type)'
# expect: server: GitHub.com + content-type: text/plain; charset=utf-8
curl -sI --max-time 10 https://www.magelift.dev/ | grep -i -E "^(server|location)"
# expect: server: GitHub.com + location: https://magelift.dev/
```
