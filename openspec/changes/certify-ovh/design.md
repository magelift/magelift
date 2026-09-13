## Context

OVH is a typed `OVHTarget` with MKS, managed MySQL, Valkey, and private-network flags. Day-2 uses shared `kube.Steps`. Bootstrap/Secrets are `ErrNotSupported`. Live bar in the packed campaign is one MKS preview, no KEEP. EU agencies still need the implemented possibility space listed.

## Goals / Non-Goals

**Goals:**
- One OVH-owned spec and matrix section for every implemented cell.
- Certified subset stays empty; experimental stays explicit.
- One preview when credits allow; destroy always.
- Campaign 6.4 OVH moves here.

**Non-Goals:**
- KEEP, certified OVH, or implementing Bootstrap/Secrets in this change.
- Inferring custom gateway routing from `privateNetworkRoutingAsDefault`.
- Cartesian live shops or spending AWS/GCP KEEP money on OVH topologies.

## Decisions

1. **Catalog = `OVHTarget` fields.** Plans, versions, node counts, floating IPs, and routing flags are the field API. Status columns: Adobe / MageLift / experimental / unavailable.

2. **Private-network honesty stays provider-specific.** Gateway, floating IP, and CNI failures are separate cells, not one “network ready” checkbox.

3. **No KEEP.** Experimental plus credit-gated preview is the live method until a later change promotes OVH.

**Alternatives considered:** Leave OVH as a packed-campaign bullet (rejected: untrackable). Certify from kube.Steps (rejected: honesty).

## Risks / Trade-offs

- Empty certified subset looks unfinished. That is accurate.
- One preview cannot cover MySQL 8.0 vs 8.4 vs enterprise plans; the catalog must still list them as implemented or unavailable.
