---
status: planned
slug: full-deployment-coverage
spec: spec.md
---

# Plan: reference onboarding for the alpha recipe

## Order of work

- [x] 1.1 SDK email settings plus plan inputs (extend `sdk.Application` with `Email`, carry through `PlanRequest`, provider-side mode validation) — verify: `GOWORK=off go test ./...` from `sdk/` exits 0
- [x] 1.2 GCP SMTP wiring (spec email, deploy-time Secret Manager resolution mirroring `resolveEncryptionKey`, Magento `CONFIG__SMTP__*` env, K8s secret, redaction tests) — verify: `go test ./providers/gcp/stack/ ./providers/gcp/runtime/` green with SMTP cases
- [x] 2.1 Core tem/ovh rejection plus budget label (validateCloudEmail unimplemented errors, provider-neutral label text, `make generate` + `generate-check` clean) — verify: config suite green, generation gates green
- [x] 2.2 Init GCP starter matches alpha (remove search-disabled pin, add staticContent, commented SMTP plus prerequisites, schema-validated) — verify: init starter tests green
- [x] 3.1 Doctor gcloud guidance (toolchain spec for GCP targets, next-action text) — verify: doctor suite green
- [x] 3.2 Classified deploy failures (PluginError code mapping at deploy/destroy boundary with scripted-session tests) — verify: cli suite green
- [x] 4.1 Skills match CLI (rollback flags, sidecar name, cost-live GCP note) — verify: skills suites green
- [x] 4.2 Security posture trace (each onboarding claim pinned to a manifest or removed) — verify: trace recorded in report
- [x] 5.1 Onboarding doc plus website pointers (new `docs/onboarding.md`, getting-started link, website install honesty, humanizer plus marks) — verify: `make docs` green, prose passes recorded
- [ ] 5.2 Full proof (root + sdk + provider + synthetic + harness suites green, gates green) — verify: recorded commands all exit 0

## Notes

- SES files untouched (complete AWS path, verified by research).
- No new CLI commands; verification reuses exec/health/logs.
- Preview email stays disabled by default (existing core behavior).
- Spec approved per autonomous goal pattern (recommended option); decisions above.
