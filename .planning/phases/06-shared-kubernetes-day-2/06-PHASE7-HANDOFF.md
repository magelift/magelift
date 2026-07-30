# Phase 7 handoff — Cloudflare DNS + live GKE

**From:** Phase 6 (Shared Kubernetes Day-2)  
**To:** Phase 7 (MIGRATE-04 DNS cutover + managed dump + live GKE Autopilot certification)  
**Date:** 2026-07-30  
**Spend:** Phase 6 closed with **zero cloud spend**. Live cells are Phase 7 only (D-06).

## What Phase 6 proved (offline)

- Shared `*kube.Observe` and `*kube.Steps` across gcp ops, eksops, ovh stack, scaleway stack (`TestFourModuleTypeIdentity`, `TestStepsSequence`)
- S3-compatible DIY State with `EncryptionAES256` for OVH/SCW (unit fake S3API); Floci AES256 under `MAGELIFT_FLOCI=1` / `make floci-test` when Docker is healthy
- Honesty matrix: Bootstrap/Secrets/Cost remain loud `ErrNotSupported` on experimental OVH/SCW; AcquireLock → State.Lock

Do **not** claim live GKE/OVH/SCW Magento acceptance from Phase 6 evidence.

## Cloudflare DNS (MIGRATE-04) — Phase 7 implements

| Item | Value |
| --- | --- |
| Preferred preview host | `magelift-preview.alexandrecourtiol.com` |
| Fallback host | `magelift-preview.acourtiol.com` |
| Zones | Authorized (2026-07-30): any subdomain of `alexandrecourtiol.com` or `acourtiol.com` |
| Required token scope | **Zone → DNS → Edit** (and Zone → Zone → Read) on those zones |
| Insufficient auth | Wrangler OAuth is **zone:read only** — list zones works; DNS create/update fails. Do not rely on Wrangler OAuth for cutover writes. |

### Operator setup (Phase 7)

```bash
export CLOUDFLARE_API_TOKEN=...   # or CF_API_TOKEN — Zone.DNS Edit
export MAGELIFT_CUTOVER_HOST=magelift-preview.alexandrecourtiol.com
TARGET=<applicationURL-or-LB> ./scripts/cutover-dns-cloudflare.sh
./scripts/cutover-dns-cloudflare.sh --cleanup
```

Script: `scripts/cutover-dns-cloudflare.sh` (offline `--dry-run` + `scripts/acceptance/cutover-dns-cloudflare_test.sh`). Do not commit tokens. Cleanup: delete the cutover CNAME/A after rehearsal.

Scratch notes from Phase 6 discuss: `.planning/phases/06-shared-kubernetes-day-2/scratch/cloudflare-dns-auth.md`.

## Explicitly out of Phase 6

- No Cloudflare DNS create/update code
- No live GKE Autopilot exercise
- Managed dump cell rides the **GCP certification pass** in Phase 7 (not implemented here)

## Resume checklist for Phase 7

1. Issue Cloudflare API token with Zone.DNS Edit on both zones
2. Point preview host at the stack ingress / load balancer from the paid pass
3. Run MIGRATE-04 cutover rehearsal; destroy DNS + stack when done
4. Certify live GKE Observe/Steps/health against the shared kube layer proven offline in Phase 6
