# Publishing and Community Launch decisions

Decisions for the public v1.0.0 (Community Launch) surface. Capability for AWS
ECS Fargate + GCP GKE Autopilot is already certified; this milestone is
distribution, trust, and narrative - not a new cloud-spend era.

## Spine

**Launch polish first.** Defer ECS Managed Instances, live OpenSearch SigV4,
Cloud SQL attach, FinOps SaaS, fifth provider, and signed plugins to
[post-beta / v1.1+](post-beta-roadmap.md).

## Domains and static-site stack

| Surface | Default | Stack |
| --- | --- | --- |
| Marketing + docs | `https://magelift.dev/` (docs at `/docs/`) | Astro landing + MkDocs Material → Cloudflare Pages project `magelift` |
| Optional docs host | `docs.magelift.dev` → redirect or same Pages custom domain | Prefer path `/docs/` as canonical |
| Repo | `https://github.com/magelift/magelift` | Source of truth for releases |

Build locally: `cd websites/marketing && npm run build:site` → deploy `dist/`.

## Optional paid cells (OpenSearch SigV4 vs Cloud SQL attach)

**Neither rides the Community Launch tag.** Both stay on the post-beta /
v1.1+ table. Public-tag substitute for OpenSearch remains the offline SigV4 +
free-tier matrix path in [release-readiness.md](release-readiness.md). AWS
brownfield attach ships; Cloud SQL attach does not block v1.0.0.

## Evidence

Committed acceptance samples live under [evidence/](evidence/README.md). Do not
re-introduce private planner directories into this public remote. Never commit
real AWS account IDs, GCP project IDs, or personal domains - use env vars
(`MAGELIFT_GCP_PROJECT`, `MAGELIFT_CUTOVER_HOST`) locally.

## History scrub (account IDs)

`main` was orphan-squashed again (2026-08-03) so maintainer AWS account
numbers, GCP project IDs, personal domains, and related email addresses are
**not reachable from `refs/heads/main`**. A private bundle of the pre-scrub
repo (all refs) lives only on the maintainer machine under
`~/.agents/archives/` - do not push it.

**Residual:** GitHub keeps `refs/pull/*/head` for closed PRs; those tips can
still contain pre-scrub blobs for anyone with repo read access. To finish
purging, ask GitHub Support to clear cached PR refs, or recreate the private
repo after exporting issues. Default `git clone` does not fetch `refs/pull/*`.

## Launch checklist (remaining ops)

1. **Site hosting:** Cloudflare Pages project `magelift` on `magelift.dev` (marketing + `/docs/`). Workflow `.github/workflows/docs.yml` still builds MkDocs artifacts; prefer `websites/marketing` `build:site` for the public deploy.
2. Cut `v1.0.0-rc.1` when [release-readiness](release-readiness.md) gates stay Closed and hosted CI is green on `main`
3. Seed ≥3 `good first issue` items ([github-labels.md](github-labels.md)) - labels created; issues opened on GitHub
4. Verify private vulnerability reporting is on (Security → Reporting) - may require UI if API is blocked
5. Keep `magelift.dev` / `www.magelift.dev` on Cloudflare Pages custom domains
6. Settings → Actions → General: allow workflows to create pull requests (needed for Release Please after history rewrite)
7. Make the repository public when the quality bar on this checklist holds
