# Report: reference onboarding for the alpha recipe

verdict: pass

## What shipped

- SDK email settings plus GCP SMTP relay wiring (Secret
  Manager password, Magento env bindings, SecretKeyRef).
- Core TEM/OVH managed modes rejected as unimplemented for
  alpha; budget labeled planning-alerts-only.
- GCP starter matches the alpha recipe (static content,
  recipe search, commented SMTP block).
- Doctor guides gcloud for GCP; deploy failures classified
  into stable exits via typed plugin errors.
- Skills match the CLI (rollback flags, migrate sidecar,
  cost --live GCP note); security posture traced per
  provider in architecture.md.
- New docs/onboarding.md plus getting-started and website
  pointers; humanizer-reviewed, marks clean, docs build
  green.

## Evidence

- Root suites: 107 packages ok, zero failures.
- Provider suites (providers/gcp): all ok.
- Synthetic: 13 passed.
- SDK, skills-test, acceptance-harness-test, docs build:
  green.
- generate-check, cli-docs-check, fmt-check, lint
  (0 issues), license-check, check-clean-room,
  workflow-check: green.
- CLI reference regen verified against real flag
  definitions; gofmt churn import-order only; lint
  orphans (gcpKubernetesMinor, effectiveScalewayRegion)
  removed with tests green.

## Deviations

- SMTP verify uses the Magento admin forgot-password flow
  over `magelift exec`, not new CLI commands.
- DNS documented as fully operator-managed: the GCP stack
  creates no DNS resources, so the audit's automation
  correction lands as an explicit manual procedure.
- Cron verified via `magelift logs --service cron`, which
  resolves to the cron Deployment.

## Follow-ups

None. Next: Order 7 verified-provider-distribution.

## Correction (2026-09-17, alpha review R03/R12)

The recipe was documented, not demonstrated. R03: GCS media is now
real (HMAC service account, prefix-scoped reads, Magento AwsS3
remote-storage config via env template, provider export/import ops,
CLI routed through the provider); live upload/view/replace/restore
proof lands in the order-8 re-run. R12: onboarding rewritten to the
shipped path (installer, provider install, image build), ADC-first
credentials with doctor detection, test-customer mail procedure, and
scheduled expiry; the pin sheet stays proposed until loop evidence
lands. Fixed in intent/alpha-review-corrections (boxes 3.1, 4.2).
