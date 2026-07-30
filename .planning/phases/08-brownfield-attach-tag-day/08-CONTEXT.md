# Phase 8: Brownfield Attach & Tag Day - Context

**Gathered:** 2026-07-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Adopt existing VPC and managed database into a MageLift stack without replacing/destroying them; refuse destructive mutate of adopted resources; document attach/detach limits; close or defer every release-readiness / REQUIREMENTS row for tag day. Floci for import mechanics; one small free-tier AWS confirmation when ADC available. GCP certification (Phase 7 live) may still be in flight — do not claim GCP certified until 07 evidence lands; RELEASE-05 can close other rows and leave GCP as Pending→07 until then.

</domain>

<decisions>
## Implementation Decisions

### Attach surface
- **D-01:** Config references existing VPC / DB by ID/ARN (or provider-native identifiers); `preview` reports import/adopt not create; apply adopts without replace. — **Reversibility:** one-way once operators learn the config shape
- **D-02:** Adopted resources MageLift does not own: any replace/destroy attempt fails before mutate with a message naming the resource (test-asserted). — **Reversibility:** one-way honesty contract
- **D-03:** Document attach limits + detach path; verify detach leaves cloud resource intact. — **Reversibility:** reversible docs

### Proof tiers
- **D-04:** Floci (or Pulumi mocks) prove import mechanics offline first. One free-tier AWS VPC+RDS adopt confirmation when credentials available (spend map pass 3 of 3). — **Reversibility:** N/A
- **D-05:** Tag day: every `docs/release-readiness.md` row Closed with evidence or Deferred with reason; every REQUIREMENTS row Complete or recorded deferral. Phase 1 hosted CI = Deferred (Act-only until GH minutes). Phase 7 GCP rows remain Pending until 07-06/07 evidence — do not fake-certify. — **Reversibility:** costly — tag board is the public honesty surface

### the agent's Discretion
- Exact YAML field names for existing VPC/DB refs (align schema)
- Whether detach is CLI command vs documented manual un-adopt

</decisions>

<canonical_refs>
- ROADMAP Phase 8 SC1–SC5
- REQUIREMENTS ATTACH-01..04, RELEASE-05
- ADR brownfield attach notes / docs/release-readiness.md
- Phase 7 STATUS: offline 07-01..05 done; live 07-06 blocked on gcloud ADC + CLOUDFLARE_API_TOKEN

</canonical_refs>

<code_context>
## Existing Code Insights
- Pulumi import patterns may exist partially — RESEARCH must map
- Floci for AWS offline
- Gate board already tracks many Closed/Deferred rows

</code_context>

<specifics>
- [--auto] Locked from ROADMAP; honesty on Phase 7 dependency for GCP certify row
- Maintainer: Act-only CI until minutes; Cloudflare zones authorized (token still needed for 07 DNS)

</specifics>

<deferred>
- Fake-certifying GCP before live pass
- Multi-cloud attach beyond AWS free-tier confirmation this milestone

</deferred>
