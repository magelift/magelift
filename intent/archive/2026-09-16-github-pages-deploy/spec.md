---
status: done
slug: github-pages-deploy
intent: intent.md
---

# Spec: GitHub Pages single deploy story

## Requirements

### Requirement: Live host is GitHub Pages before any deletion

The change SHALL record passing live-host evidence that GitHub Pages (not Cloudflare) serves `https://magelift.dev/` before deleting any file.

#### Scenario: Primary live check passes

- **WHEN** `curl -sI --max-time 10 https://magelift.dev/ | grep -i server` is run
- **THEN** the output is exactly `server: GitHub.com`
- **AND** `curl -sI --max-time 10 https://magelift.dev/` contains `x-github-request-id:` and no `cf-ray:` header
- **AND** `dig +short www.magelift.dev CNAME` returns `magelift.github.io.`
- **AND** `dig +short magelift.dev A` returns addresses in the `185.199.108.153` / `185.199.109.153` / `185.199.110.153` / `185.199.111.153` GitHub Pages range

#### Scenario: Sandbox has no network

- **WHEN** `curl -sI --max-time 10 https://magelift.dev/` fails with a network error (exit 6 or 28)
- **THEN** the applier records `live-check: network-unavailable` with the exit code and instead verifies `gh api repos/magelift/magelift/pages --jq '.cname'` returns `magelift.dev`
- **AND** deletions SHALL NOT proceed until one of the two paths passes

### Requirement: Cloudflare-origin `_headers` and `_redirects` deleted

The change SHALL delete `website/public/_headers` and `website/public/_redirects`.

#### Scenario: Files gone from tree and index

- **WHEN** `test ! -e website/public/_headers && test ! -e website/public/_redirects` is run
- **THEN** it exits 0
- **AND** `git ls-files website/public/ | grep -E '_headers|_redirects'` returns empty

#### Scenario: Built output carries no residue

- **WHEN** `website/scripts/build-site.sh` completes
- **THEN** `ls website/dist/_headers website/dist/_redirects` fails (no such files)
- **AND** `grep -rn "_headers\|_redirects" website/scripts/build-site.sh website/astro.config.mjs` returns empty, proving nothing referenced them

### Requirement: `.cloudflare/` cache untracked and ignored without leaking values

The change SHALL remove `.cloudflare/cache/cloudflare-account.json` from git tracking, ignore `.cloudflare/`, and confirm the file holds account metadata only without printing values.

#### Scenario: Untracked plus ignored

- **WHEN** `git ls-files .cloudflare/` is run after the change
- **THEN** it returns empty
- **AND** `grep -F '.cloudflare/' .gitignore` exits 0 (placed next to the existing `.wrangler/` entry)

#### Scenario: Pre-delete shape check prints types, never values

- **WHEN** the applier runs `python3 -c "import json;d=json.load(open('.cloudflare/cache/cloudflare-account.json'));print(sorted(d.keys()));print({k:type(v).__name__ for k,v in d.items()})"` before removal
- **THEN** the output shows only key names and type names (research observed a single top-level `account` object: account metadata, no tokens)
- **AND** no command in the change prints the file's raw contents or any value containing an ID, token, or key

### Requirement: Redundant `docs.yml` deleted with no dangling references

The change SHALL delete `.github/workflows/docs.yml`.

#### Scenario: File gone, no references remain

- **WHEN** `test ! -e .github/workflows/docs.yml` is run
- **THEN** it exits 0
- **AND** `grep -rln "docs\.yml" .github/ docs/ website/ contrib/ Makefile scripts/ | grep -v "mkdocs\.yml"` returns empty (the bare pattern also matches `mkdocs.yml` substrings, so those are filtered; any remaining hit names the deleted workflow)
- **AND** `grep -rln "mkdocs-site" .github/ docs/ website/ contrib/ Makefile scripts/` returns empty
- **AND** `grep -rn "Docs site" .github/` returns empty

#### Scenario: No required check points at the deleted workflow

- **WHEN** `gh api repos/magelift/magelift/branches/main/protection --jq '.required_status_checks.contexts[]'` is run (repo-side gate is the `CI passed` aggregation in `.github/workflows/ci.yml`, not `Docs site`)
- **THEN** no returned context equals `Docs site`
- **AND** if the API is unavailable, the applier records a manual branch-protection UI check with the same expected result before merging

### Requirement: Skill Deploy section documents Pages only

The `## Deploy` section of `contrib/skills/magelift-site/SKILL.md` SHALL document GitHub Pages via `.github/workflows/site.yml` with no wrangler or Cloudflare deploy path.

#### Scenario: No wrangler residue, Pages wording present

- **WHEN** `grep -in "wrangler\|cloudflare" contrib/skills/magelift-site/SKILL.md` is run
- **THEN** it returns empty
- **AND** `grep -F "npx wrangler pages deploy dist --project-name=magelift --branch=main" contrib/skills/magelift-site/SKILL.md` returns empty (the exact deleted line)
- **AND** `grep -F ".github/workflows/site.yml" contrib/skills/magelift-site/SKILL.md` exits 0
- **AND** `grep -F "actions/deploy-pages" contrib/skills/magelift-site/SKILL.md` exits 0

### Requirement: README and publishing notes agree on Pages

`website/README.md` and `docs/publishing.md` SHALL state GitHub Pages via the Public site workflow as the sole deploy story with no Cloudflare alternative.

#### Scenario: Both pages agree (verify, edit only if drifted)

- **WHEN** `grep -F "GitHub Pages" website/README.md` and `grep -F "GitHub Pages" docs/publishing.md` are run
- **THEN** both exit 0
- **AND** `grep -in "wrangler\|cloudflare" website/README.md docs/publishing.md` returns empty
- **AND** any edit to these human pages goes through the humanizer skill, then the remove-ai-marks skill, before the diff is final

### Requirement: `CNAME` kept byte-identical

The change SHALL keep `website/public/CNAME` containing `magelift.dev`.

#### Scenario: Apex domain file untouched

- **WHEN** `test "$(cat website/public/CNAME)" = "magelift.dev"` is run
- **THEN** it exits 0
- **AND** after `website/scripts/build-site.sh`, `test "$(cat website/dist/CNAME)" = "magelift.dev"` exits 0

### Requirement: Site build stays green

The change SHALL keep the site build green via `website/scripts/build-site.sh`.

#### Scenario: Full build passes

- **WHEN** `website/scripts/build-site.sh` is run from the repo root
- **THEN** it exits 0
- **AND** `test -d website/dist/docs` exits 0 (MkDocs `--strict` output embedded)
- **AND** `test -f website/dist/CNAME` exits 0

#### Scenario: Offline fallback (npm registry unreachable only)

- **WHEN** `website/scripts/build-site.sh` fails solely at the `npm ci` / `npm install` step with a registry network error
- **THEN** `"$("scripts/docs-venv.sh")/mkdocs" build --strict --config-file mkdocs.yml --site-dir /tmp/magelift-docs-probe` exits 0 as the documented subset
- **AND** the npm failure output is recorded verbatim in the change report; a green subset does not excuse an unexamined npm failure

### Requirement: No URL changes

The change SHALL NOT alter the site's public URLs: `magelift.dev` apex, `/docs/` docs path, `www` apex redirect.

#### Scenario: URL-bearing config untouched

- **WHEN** `grep -F "site: 'https://magelift.dev'" website/astro.config.mjs` and `grep -F "site_url: https://magelift.dev/docs/" mkdocs.yml` are run
- **THEN** both exit 0
- **AND** `grep -F "https://magelift.dev/" contrib/skills/magelift-site/SKILL.md` exits 0
- **AND** `git diff --exit-code -- website/astro.config.mjs mkdocs.yml website/public/CNAME` exits 0

### Requirement: Deleted redirect and MIME config has live coverage

Each behavior the deleted files claimed SHALL have recorded live or in-tree coverage proving the deletion loses nothing.

#### Scenario: Redirects covered

- **WHEN** `grep -F 'url=/docs/getting-started/' website/public/getting-started/index.html` is run
- **THEN** it exits 0 (the `/getting-started` meta-refresh + canonical survives; the `_redirects` line was redundant)
- **AND** recorded live evidence shows `curl -sI --max-time 10 https://magelift.dev/docs` returning `server: GitHub.com` with `location: https://magelift.dev/docs/` (Pages serves the `/docs` to `/docs/` 301 natively)

#### Scenario: MIME types covered without `_headers`

- **WHEN** recorded live evidence shows `curl -sI --max-time 10 https://magelift.dev/llms.txt`
- **THEN** it contains `server: GitHub.com` and `content-type: text/plain; charset=utf-8`, proving Pages already serves the correct type with no header forcing
- **AND** `curl -sI --max-time 10 https://www.magelift.dev/ | grep -i -E "^(server|location)"` shows `server: GitHub.com` and `location: https://magelift.dev/`, proving the www-to-apex behavior the `_redirects` comment alluded to

## Design

### File operations (exact)

| Path | Op | Detail |
| --- | --- | --- |
| `website/public/_headers` | DELETE | Cloudflare-only path-block syntax (19 lines: `/llms.txt`, `/llms-full.txt`, `/pricing.md`, `/robots.txt`, `/sitemap.xml`); GitHub Pages ignores it. `git rm`. |
| `website/public/_redirects` | DELETE | Netlify/Cloudflare-only syntax; opens with an invalid `#!/usr/bin/env bash` first line. Do not fix, delete. `git rm`. |
| `.cloudflare/cache/cloudflare-account.json` | DELETE from index + working tree | Tracked cache; regenerable. `git rm .cloudflare/cache/cloudflare-account.json` (only file under `.cloudflare/` per `git ls-files .cloudflare/`). Run the keys/types-only shape check first (see requirement scenario), never `cat`. |
| `.gitignore` | EDIT (one line) | Append `.cloudflare/` next to the existing `.wrangler/` entry so the cache never re-enters. |
| `.github/workflows/docs.yml` | DELETE | Redundant (safety case below). `git rm`. |
| `contrib/skills/magelift-site/SKILL.md` | EDIT (`## Deploy` only) | Replace the wrangler block with the Pages wording below. Leave `## Stack` (already Pages-correct), `## Copy rules`, `## Use this skill when`, `## Leave behind` untouched. |
| `website/README.md` | VERIFY, edit only on drift | Already correct (line 28: Public site workflow deploys `dist/` to GitHub Pages). |
| `docs/publishing.md` | VERIFY, edit only on drift | Already correct (Build row line 25, launch checklist line 45: Site on GitHub Pages Done). |
| `website/public/CNAME` | KEEP | `magelift.dev`, byte-identical. |
| `.github/workflows/site.yml` | KEEP | Sole deploy owner; no edits needed. |
| `website/scripts/build-site.sh` | KEEP | No references to `_headers`/`_redirects`; Astro copies `public/` verbatim so deletion propagates to `dist/` automatically. |

### Replacement skill Deploy section (verbatim)

Replace lines 35-43 of `contrib/skills/magelift-site/SKILL.md`:

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

Every claim above is evidenced: `site.yml` deploy job is gated on `github.event_name == 'push' && github.ref == 'refs/heads/main'` (line 54); `www` 301 to apex and `/docs` 301 to `/docs/` observed live from `server: GitHub.com`.

### `llms.txt` MIME intent: accept sniffing (nothing lost)

`_headers` forced `Content-Type: text/plain; charset=utf-8` on `/llms.txt` and `/llms-full.txt`. That file is dead on Pages — but live evidence shows Pages already serves `content-type: text/plain; charset=utf-8` for `/llms.txt` natively, so deletion changes zero observable behavior; no Astro-native replacement is needed or wanted. Browsers would sniff `.txt` as text regardless. No new config file replaces `_headers`.

### `docs.yml` deletion safety case

Three independent covers, all verified in-tree:

1. **PR strict-build is owned by `ci.yml`, not `docs.yml`.** The `docs` job (`.github/workflows/ci.yml` lines 192-207) runs `mkdocs build --strict` on PRs and pushes gated by the `docs` path filter (`docs/**`, `mkdocs.yml`). `docs.yml` has no `pull_request` trigger at all — it never gated a PR, so deleting it removes zero PR signal.
2. **`main` docs coverage is owned by `site.yml`.** Its `build` job runs `website/scripts/build-site.sh`, whose line 14 runs `mkdocs build --strict`, on the same `docs/**` + `mkdocs.yml` + `docs/requirements.txt` paths (plus `website/**`). Every `main` push `docs.yml` would have built, `site.yml` also builds — strictly.
3. **The `docs.yml` artifact is unconsumed.** It uploads `mkdocs-site` with 14-day retention; no workflow, script, Makefile target, or doc references `mkdocs-site` (grep-verified). Deleting the producer strands nothing. The header comment claiming Pages is unavailable is stale: `docs/publishing.md` records Pages Done and live headers prove it.

### Verification ordering

1. Live-host check first (Requirement 1, primary or fallback path recorded).
2. `.cloudflare/` shape check second (types only), then `git rm` + `.gitignore` edit.
3. Deletions (`_headers`, `_redirects`, `docs.yml`) and the skill edit.
4. No-dangling-refs greps.
5. Full `website/scripts/build-site.sh` (or documented subset with recorded npm failure).
6. Post-merge observation of the next `main` Pages deploy (see Gotchas).

## Gotchas / policy flags

- **Deploy-continuity risk.** The next `main` push must still ship the site. Pre-merge signal: the PR build of `site.yml` (build-only on PRs) must pass. Post-merge observation (owner: merger): watch the Pages deployment via `gh run list --workflow=site.yml --branch main` / the `github-pages` environment, then re-run `curl -sI --max-time 10 https://magelift.dev/ | grep -i server` expecting `server: GitHub.com`. If the deploy fails, the change regressed continuity and takes priority over everything else in this spec.
- **Humanizer + remove-ai-marks.** Any touched human pages (`website/README.md`, `docs/publishing.md` — both expected no-ops) go through the humanizer skill, then the remove-ai-marks skill. The skill Deploy section is agent surface, but keep it plain and factual per copy rules; do not invent certified claims, mirror `docs/capability-matrix.md`.
- **Acceptance Cloudflare DNS helpers are out of scope — do not touch.** All of these are DNS-cutover tooling for acceptance cells, unrelated to site hosting: `scripts/acceptance/lib-cloudflare-dns.sh`, `scripts/cutover-dns-cloudflare.sh` (including the `wrangler whoami` hint at line 82), `tests/acceptance/cloudflare_dns_helper_test.sh`, `tests/acceptance/gcp_harness_shape_test.sh`, `tests/acceptance/gcp_edge_harness_shape_test.sh`, `tests/acceptance/aws_cloudfront_harness_shape_test.sh`, `scripts/gcp-acceptance-local.sh`, `scripts/gcp-edge-acceptance-local.sh`, `scripts/fastly-acceptance-local.sh`, `scripts/aws-cloudfront-acceptance-local.sh`, and the `Makefile` harness line invoking `cloudflare_dns_helper_test.sh`. Verify greps MUST scope to `.github/ docs/ website/ contrib/` (plus `Makefile`, `scripts/` only for `_headers`/`_redirects`/`docs.yml` patterns) so these files never appear as false positives.
- **No secrets in `.cloudflare/`.** Research confirmed type only: JSON object with a single top-level `account` object (account metadata). The applier re-confirms with the keys/types-only query before `git rm`; never print values, never paste the file into evidence or logs.
- **Branch protection blind spot.** Required checks live in GitHub settings, not the repo. The repo-side gate is the `CI passed` aggregation, but if an admin ever added `Docs site` as required, deletion breaks the merge queue. The `gh api .../protection` scenario covers this; owner for removal is the repo admin.
- **Do not "fix" `_redirects`.** The `#!/usr/bin/env bash` first line is invalid redirect syntax, but the correct action is deletion (Pages ignores the file), not a syntax repair that would imply the file matters.
- **Smallest correct change.** No `site.yml` edits, no `build-site.sh` edits, no Astro config changes, no new header/redirect mechanism, no URL changes.

## Open questions carried forward

- **Is any live Cloudflare Pages project still serving magelift.dev?** Resolved: no. Live evidence recorded 2026-09-15 in-spec: apex serves `server: GitHub.com` with `x-github-request-id`, no `cf-ray`; apex A records are Pages IPs; `www` CNAMEs to `magelift.github.io.`; `/llms.txt` MIME, `/docs` 301, and www-to-apex 301 all served by GitHub. Default (Pages sole host) confirmed. Owner: applier re-runs the one-line curl at apply time as a cheap freshness check; if fingerprints flip to `server: cloudflare` + `cf-ray`, stop and re-open the intent instead of deleting.
- **Should the `llms.txt` MIME forcing be replaced?** Default accepted: no replacement. Strengthened by evidence — Pages already serves `text/plain; charset=utf-8` for `/llms.txt`, so nothing is lost and sniffing is not even load-bearing. Owner: none; no action.
- **Is `Docs site` a required branch-protection check?** Unverifiable from the repo; default assumption is no (the documented gate is `CI passed`). Owner: applier runs the `gh api` protection scenario (or a repo admin checks the UI) before merging; if present, the admin removes it as part of this change.
