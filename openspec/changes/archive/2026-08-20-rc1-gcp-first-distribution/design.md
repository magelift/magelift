## Context

See proposal.md for motivation. Today first-party adapters live under
`internal/cloud/<provider>` and `internal/external`, linked into one Go module.
`sdk/v1.Module` is documented as build-time only. CI pins Floci `1.5.33`.
`docs/gcp-experimental.md` denies floci-gcp. Live tasks in
`provider-native-lifecycle-adapters` and `multi-cloud-resilience-observability-edge`
keep a checkbox open until every provider has a paid cell. ADR 0007 still
describes AWS as v1 certified and GCP as v1.1, and rejects default remote plugin
install.

## Goals / Non-Goals

**Goals:**

- Dual-mode loading of the same `sdk.Module` packages (in-process tests, signed
  subprocess published CLI).
- Digest-pinned Floci AWS 1.7.0 and floci-gcp 0.7.0 in CI in parallel.
- Packed KEEP sessions: GCP Magento origin plus integrations, then AWS Magento,
  then OVH/Scaleway infra-only.
- Task-file split so GCP evidence can close without OVH/AWS leftovers.
- ADR 0011 superseding ADR 0007's certification order and plugin default.

**Non-Goals:**

- Certified Magento on OVH or Scaleway in RC1.
- Re-proving Fastly, New Relic, Cloudflare, or SendGrid on AWS.
- Go `plugin.Open`.
- Completing the Go workspace extract and HashiCorp go-plugin host in the same
  commit as Floci pins; Magento cells stay in-process until a GCP host smoke
  exists.
- floci-az.

## Decisions

1. **HashiCorp go-plugin over Go plugins and over a custom JSON protocol.**
   Same process isolation Terraform uses, gRPC, Windows-safe. Alternative
   considered: `plugin.Open` (rejected: ABI, Windows). Alternative: fat binary
   forever (rejected: size and community). Alternative: wait to tag until eight
   subprocesses extract (rejected: TTM). In-process fallback is a one-release
   ceiling.

2. **Monorepo Go workspace, not separate git repos.** Packages:
   `providers/{gcp,aws,ovh,scaleway}` and
   `integrations/{cloudflare,sendgrid,newrelic,fastly}`. Core CLI stops importing
   cloud SDKs in the published artifact. Tests import packages directly.

3. **Emulator vs live split.** floci-gcp covers GCS, Secret Manager, Pub/Sub,
   Logging, Monitoring, IAM/STS contracts. Live remains required for Autopilot,
   Memorystore, Armor, managed TLS, Magento Cloud SQL PITR, and HA known-content.
   Pin Hub tags `floci/floci:1.7.0` and `floci/floci-gcp:0.7.0` by manifest
   digest at apply time (re-verify; 2026-08-18 indexes were
   `sha256:bec9b9f749322444a57fb0f00d3e007ffd49015162361aab84ccecd9b4f5f8ed` and
   `sha256:9f1a020fdd5ac8b6ca568b60e793a9a0e88b8df400ae6bd68283ec5208788180`).

4. **Packed sessions instead of per-checkbox destroy.** Session 1 GCP KEEP
   Magento; session 2 AWS KEEP Fargate; session 3 OVH/SCW infra concurrent with
   2 when isolated. GCP KEEP allowed (spend ignored). AWS KEEP only for session
   2 duration.

5. **Ownership of remaining checkboxes.** `provider-native-lifecycle-adapters`
   owns adapter impl plus the first bounded live cell of a *new* API.
   `multi-cloud-resilience-observability-edge` owns the evidence catalog and
   release matrix. Split leftover rows into `impl` / `floci` / `live.gcp` /
   `live.aws` / `live.integration` / `live.ovh-scw`. Close typed-unsupported
   fencing, alternate-region DR, lost-credentials, and provider-outage on
   `gcp.gke`.

6. **ADR 0011 supersedes ADR 0007** on certification order (GCP then AWS for
   Magento), RC1 adapter set, signed subprocess install, and floci-gcp as a
   first-class emulator. ADR 0007's compile-time community path remains valid
   as `examples/custom-cli`.

## Risks / Trade-offs

- [Floci 1.7.0 or floci-gcp 0.7.0 breaks an existing AWS contract] → Keep the
  AWS suite green before expanding; record UnsupportedOperation as typed gaps,
  do not weaken production IAM.
- [floci-gcp Cloud SQL is not Magento MySQL PITR] → Do not move Magento DB
  restore off live GCP.
- [Plugin host delays Magento] → Host work is parallel and non-gating; RC1 may
  ship leftover adapters in-process for one release.
- [KEEP on AWS overruns credits] → Destroy at session 2 end; no KEEP across
  days.

## Migration Plan

1. Pin Floci images and add floci-gcp GCS smoke + CI job (done). Remaining
   floci-gcp APIs are optional and must not block KEEP.
2. Rewrite BACKLOG so pre-KEEP local CLI (health layers, dump-from-live,
   rollback-schema, SendGrid config validation) unblocks session 1. Split
   in-progress task files. Signed host stays parallel, never after session 3.
3. Run packed session 1, then 2, then leftover OVH/SCW infra. Session 1 uses
   the existing failed-deployment injector via `MAGELIFT_GCP_FAILED_DEPLOY_DIGEST`.
4. Extract `magelift-provider-gcp` behind the host in parallel with Magento
   cells; in-process fallback is a one-release ceiling.
5. Tag `v1.0.0-rc.1`. Rollback is leaving adapters in-process; lockfile
   install is additive.
