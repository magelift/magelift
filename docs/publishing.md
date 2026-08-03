# Publishing and Community Launch decisions

Decisions for the public v1.0.0 (Community Launch) surface. Capability for AWS
ECS Fargate + GCP GKE Autopilot is already certified; this milestone is
distribution, trust, and narrative — not a new cloud-spend era.

## Spine

**Launch polish first.** Defer ECS Managed Instances, live OpenSearch SigV4,
Cloud SQL attach, FinOps SaaS, fifth provider, and signed plugins to
[post-beta / v1.1+](post-beta-roadmap.md).

## Domains and static-site stack

| Surface | Default | Stack |
| --- | --- | --- |
| Docs | `docs.magelift.com` | Existing MkDocs (`mkdocs.yml`) → GitHub Pages or Cloudflare Pages |
| Marketing | `www.magelift.com` (apex → www) | Astro static site (separate from MkDocs) |
| Repo | GitHub (`magelift` org when quality bar met) | Source of truth for releases |

Exact DNS / CDN cutover is a launch checklist item; content work can start on
GitHub Pages / preview URLs before custom domains resolve.

## Optional paid cells (OpenSearch SigV4 vs Cloud SQL attach)

**Neither rides the Community Launch tag.** Both stay on the post-beta /
v1.1+ table. Public-tag substitute for OpenSearch remains the offline SigV4 +
free-tier matrix path in [release-readiness.md](release-readiness.md). AWS
brownfield attach ships; Cloud SQL attach does not block v1.0.0.

## Evidence

Committed acceptance samples live under [evidence/](evidence/README.md). Do not
re-introduce private planner directories into this public remote. Never commit
real AWS account IDs, GCP project IDs, or personal domains — use env vars
(`MAGELIFT_GCP_PROJECT`, `MAGELIFT_CUTOVER_HOST`) locally.

## Launch checklist (remaining ops)

1. **Docs hosting:** GitHub Pages is unavailable on this private plan — publish MkDocs via **Cloudflare Pages** (or make the repo public, then enable Pages). Workflow `.github/workflows/docs.yml` stays ready for Pages when the plan allows it; until then build with `mkdocs build --strict` in CI and deploy `site/` from Cloudflare connected to `main`.
2. Cut `v1.0.0-rc.1` when [release-readiness](release-readiness.md) gates stay Closed and hosted CI is green on `main`
3. Seed ≥3 `good first issue` items ([github-labels.md](github-labels.md)) — labels created; issues opened on GitHub
4. Verify private vulnerability reporting is on (Security → Reporting) — may require UI if API is blocked
5. Point `docs.magelift.com` / `www.magelift.com` when DNS is ready (Cloudflare Pages custom domains)
6. Move to `magelift` GitHub org when the quality bar is met (ORG-01)
7. Settings → Actions → General: allow workflows to create pull requests (needed for Release Please after history rewrite)
