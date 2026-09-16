---
status: accepted
slug: full-deployment-coverage
---
# Intent: reference onboarding for the alpha recipe

## Problem

There is no single supported path a new team can follow from a clean
workstation to a working shop on the alpha recipe. The previous scope of this
intent (managed SES plus TEM plus OVH mailbox in one change) delayed proof of
the primary workflow to build three email vendors before any shop was operable.
Agencies without DevOps capacity need one documented GCP Autopilot recipe with
honest prerequisites, one validated SMTP path, and errors that name the cause —
not three parallel email adapters.

## Evidence

`intent/audit.md` F13: the `audit.md` inventory in this directory is useful
but some conclusions are wrong or outdated — a secret reference does not mean
manual creation (MageLift can generate and store it), domain ownership does not
mean every DNS operation is manual, and managed KMS is not inherently
incompatible with automation. Conversely, billing activation, domain ownership,
licenses, and some email approvals cannot honestly promise zero human action.

`intent/audit.md` F10: website budget-cap and spend-stopped language exceeds
what the traced readers enforce (`Enforced: false`, GCP live pricing unwired).
Onboarding must report estimates, missing prices, alerts, and limits honestly.

Existing AWS onboarding exposes certificate, notification, and logging
prerequisites that need explicit treatment; the partially implemented SES files
must be deliberately completed, isolated, or removed, not left to accidentally
become the release contract.

## Proposed outcome

A new team follows one documented path and runs a real shop on the supported
GCP Autopilot recipe: install, configure from template, validate, deploy,
verify HTTPS plus assets plus search plus cron/consumers plus outbound SMTP
plus persistent media. Prerequisites that need human action (billing, domain
ownership, sandbox exit, licenses) are named pre-deploy with procedures, not
discovered at first send. One supported SMTP path is validated end to end for
alpha; additional email vendors wait for post-alpha demand. Deploy failures
print classified causes. Skills describe the path as built.

## Affected users and systems

Prospective pilot agencies and SME teams. `internal/config` (validation and
error text), onboarding docs and `examples/sample-shop`, `magelift doctor`,
`magelift init` importer behavior on the alpha recipe, user skills
(`magelift-configure`, `magelift-operate`, `magelift-migrate` as touched),
website install and onboarding copy (honest cost and responsibility language).

## Constraints

- Secrets by reference only; MageLift may generate and store secrets, never
  print values in YAML, logs, or evidence.
- YAML-only supported path: no `MAGELIFT_KUBECONFIG`-style manual overrides
  as the documented flow.
- Honest cost language: estimates, unpriced lists, alerts, and limits. No
  hard-cap or spend-stopped promise the code cannot enforce.
- Topology stays in `internal/cloud/<provider>/` (ADR 0003, 0004).
- Nothing is called certified or managed until the capability matrix plus
  evidence says so.
- Human docs and website copy go through humanizer, then remove-ai-marks.
- Runs after `gcp-autonomous-provider`: onboarding documents a provider that
  exists, not a design.

## Out of scope

- Managed SES/TEM/OVH-mailbox adapters as a bundle; one validated SMTP path
  for alpha, the rest post-alpha on demand.
- AWS onboarding gaps (ACM, SNS topic, log bucket): owned by
  `aws-provider-parity`.
- Preview-environment autonomous expiry mechanics: owned by the provider plus
  acceptance; onboarding documents the policy once it exists.
- Stable v1 freeze or provider catalog expansion.

## Open questions

- Which single SMTP path is the alpha default (operator relay via `smtp` mode
  with a documented provider, or a minimal managed path)? Default: operator
  relay with a validated procedure, unless the spec proves a managed path is
  cheaper than its support surface. Owner: spec author.
- What happens to the partially implemented SES files: complete, isolate
  behind experimental, or remove? Default: isolate or remove unless the SMTP
  decision needs them. Owner: spec author.
- Time-from-clean-workstation target for the acceptance run (carries into
  `reference-store-acceptance`)? Default: measure first, set the bar from
  pilot data. Owner: maintainer.
