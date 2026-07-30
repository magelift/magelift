# Phase 7: GCP Certification - Context

**Gathered:** 2026-07-30
**Status:** Ready for planning

<domain>
## Phase Boundary

One paid GCP pass certifies GKE Autopilot: WIF bootstrap, Secret Manager composer creds, live day-2 (logs/exec/secrets/state/health), deploy migrate→cutover→health→record, cost estimate, capability-matrix/release-readiness certified. Ride managed dump-import cell + MIGRATE-04 Cloudflare DNS rehearsal (`magelift-preview.alexandrecourtiol.com`) on the same pass. Destroy all resources (`force_clean` + PSA soak + DNS cleanup). Phase 8 attach stays out. Act-only for GitHub CI until minutes return (GCP-01 CI auth may use Act or a documented WIF dry-run + one real token exchange).

</domain>

<decisions>
## Implementation Decisions

### Project / region
- **D-01:** Use existing GCP project `digital-lab-341608` (active gcloud config). Prefer a low-cost region already used by MageLift GCP docs (confirm in RESEARCH; default `europe-west1` if docs agree). Prefix resources with harness prefix (`mlgcpwt` or docs default). — **Reversibility:** reversible project choice before first create

### Paid pass shape
- **D-02:** Single create-once harness pass (Phase 3 pattern): resume-friendly cells for day-2 commands + deploy + dump-import + cost; evidence in `.magelift/gcp-matrix/matrix-results.md`; teardown with force_clean + PSA soak. No exploratory second create. — **Reversibility:** one-way once spend starts

### Managed dump cell
- **D-03:** One harness cell: synthetic sanitized SQL fixture → `env create --dump` → after first successful deploy, assert tables in Cloud SQL + journal `imported`. Uses Phase 5 dumpimport/auto-import. — **Reversibility:** reversible fixture content

### Cloudflare DNS (MIGRATE-04)
- **D-04:** Preview host `magelift-preview.alexandrecourtiol.com` (fallback `magelift-preview.acourtiol.com`). Create/update CNAME or A via Cloudflare API using `CLOUDFLARE_API_TOKEN` / `CF_API_TOKEN` with Zone.DNS Edit — **not** Wrangler OAuth (zone:read only). Point at stack ingress/LB from the paid pass. Record cutover rehearsal; delete DNS record on cleanup. — **Reversibility:** reversible DNS records

### WIF / CI (GCP-01)
- **D-05:** Bootstrap WIF on the real project. Prove token exchange without committing SA keys. Hosted GitHub Actions minutes exhausted → prove with Act locally and/or a documented `gcloud` WIF exchange; matrix records Act-only until minutes return. — **Reversibility:** costly once WIF pools exist (must cleanup)

### Certification honesty
- **D-06:** Mark `gcp` / `gke-autopilot` certified only after evidence rows exist for SC1–SC5; update capability-matrix + release-readiness citing this pass. — **Reversibility:** costly — certification claim

### the agent's Discretion
- Exact cell order in harness resume file
- Whether dump cell runs before or after day-2 cells (prefer after create-once healthy)
- Cloudflare record type (CNAME vs A) based on what the stack exports

</decisions>

<canonical_refs>
## Canonical References

- `.planning/ROADMAP.md` Phase 7 SC1–SC5
- `.planning/REQUIREMENTS.md` GCP-01..06; MIGRATE-04 Phase 7 HUMAN_GATE
- `docs/gcp-acceptance.md`, `docs/gcp-experimental.md`
- Phase 6 `06-PHASE7-HANDOFF.md` — Cloudflare auth + shared kube
- Phase 5 dumpimport / seeddump / cutover runbook
- Phase 3 AWS harness pattern (resume, evidence, assert_clean)

</canonical_refs>

<code_context>
## Existing Code Insights

- Shared `kube.Observe` / `kube.Steps` ready for live exercise
- GCP acceptance scripts/docs exist; force_clean + PSA soak lessons in knowledge
- Dump import auto-once + `env import-dump` from Phase 5
- Cost estimator stubs exist; GCP-05 needs real estimate path

### Operator prerequisites (must be green before live create)
1. `gcloud auth login` (token refresh currently failing non-interactively)
2. `export CLOUDFLARE_API_TOKEN=...` with Zone.DNS Edit on both zones
3. Spend approved + destroy-when-done (already stated)

</code_context>

<specifics>
## Specific Ideas

- [--auto] Locked from ROADMAP + Phase 6 handoff + maintainer DNS authorization
- Preferred host: magelift-preview.alexandrecourtiol.com
- GitHub hosted CI remains Act-only until minutes return

</specifics>

<deferred>
## Deferred Ideas

- Phase 8 brownfield attach
- Multi-region GCP
- Hosted GitHub Actions green (minutes)

</deferred>
