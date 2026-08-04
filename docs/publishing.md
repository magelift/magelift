# Publishing

Notes for the public `v1.0.0` surface. AWS ECS Fargate and GCP GKE Autopilot are
already certified. This milestone is packaging, docs, and repo trust - not a new
round of paid cloud cells.

## What waits until later

Keep these on [post-beta / v1.1+](post-beta-roadmap.md):

- ECS Managed Instances
- Live OpenSearch SigV4 data-plane
- Cloud SQL attach
- FinOps SaaS, fifth provider, signed plugins

OpenSearch for the public tag still uses the offline SigV4 + free-tier path in
[release-readiness](release-readiness.md). AWS brownfield attach ships; Cloud SQL
attach does not block `v1.0.0`.

## Site and repo

| Surface | Where |
| --- | --- |
| Marketing + docs | [magelift.dev](https://magelift.dev/) (docs under `/docs/`) |
| Build | `cd website && npm run build:site` → GitHub Pages via the Public site workflow |
| Source | [github.com/magelift/magelift](https://github.com/magelift/magelift) |

Canonical docs URL is the `/docs/` path.

## Evidence and secrets

Acceptance samples: [evidence/](evidence/README.md). Never commit real AWS account
IDs, GCP project IDs, or personal domains. Use env vars locally
(`MAGELIFT_GCP_PROJECT`, `MAGELIFT_CUTOVER_HOST`).

## History

Public `main` is a clean import. Closed PR tip refs on GitHub may still hold older
blobs until Support clears them; a normal `git clone` does not fetch `refs/pull/*`.

## Launch checklist

| Item | Status |
| --- | --- |
| Site on GitHub Pages (`magelift.dev`) | Done |
| Private vulnerability reporting | Done |
| ≥3 `good first issue` items | Done - #38, #42, #44 open |
| Hosted CI green on `main` | Done - force-all [30836267745](https://github.com/magelift/magelift/actions/runs/30836267745) |
| Org setting: Actions can create PRs (Release Please) | Confirm in org Actions permissions (needs admin UI / `admin:org`) |
| Cut `v1.0.0-rc.1` | After readiness gates stay Closed and CI is green |
