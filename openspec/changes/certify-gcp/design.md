## Context

GCP YAML already exposes Autopilot vs Standard, Cloud SQL, Memorystore, OpenSearch, and queue knobs, but docs still say “preset-derived” and tracking is packed-campaign 6.2. Certified tier is Autopilot evidenced runtime cells only. `gcap29` KEEP exists as retained evidence, not a certified-row replacement until destroy.

## Goals / Non-Goals

**Goals:**
- One GCP-owned spec and matrix section for every implemented `target.gcp` cell.
- Certified subset remains Autopilot evidenced runtime.
- Operators can express field architectures without switching presets as the only API.
- Campaign 6.2 moves here; status independent of AWS KEEP.

**Non-Goals:**
- Certifying GKE Standard, HA, or Armor Magento exclusion from nearby Autopilot evidence.
- Copying AWS catalog enums (`amazon-mq`, AOSS) onto GCP.
- Cloud SQL brownfield attach or Magento Pub/Sub modules (named gaps stay named).
- Signing a 2.4.8-p5 image in this planning change.

## Decisions

1. **Presets remain defaults, not the catalog.** Omitted fields still resolve from `preview`/`standard`/`high-availability`. Explicit `target.gcp` fields are the agency API. Docs that say GCP has “no AWS-style toggles” MUST be corrected to match `GCPTarget`.

2. **Cold boundary is Autopilot vs Standard.** Switching runtime is a new baseline. Queue/search replica changes MAY be warm when the fingerprint allows.

3. **Vendors stay on GCP origin** for the packed campaign’s $50 vendor slice. This spec does not make Scaleway/OVH an origin for Fastly/Cloudflare.

4. **Adobe intersection stays honest.** Cloud SQL MySQL on 2.4.6-p15 remains topology/runtime evidence, not an Adobe-compatible certified database cell.

**Alternatives considered:** Invent a `target.gcp.catalog` nested object like AWS (extra schema, no new behavior). Keep preset-only docs (rejects the field-composition goal).

## Risks / Trade-offs

- Correcting “preset-only” copy may look like a new feature when the knobs already exist. The work is honesty plus matrix completeness, then KEEP apply.
- Autopilot cannot set `vm.max_map_count` for three-node OpenSearch; HA remaining on Standard is a real constraint, not a docs omission.
