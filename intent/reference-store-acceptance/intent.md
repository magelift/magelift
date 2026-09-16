---
status: draft
slug: reference-store-acceptance
---
# Intent: reference-store acceptance (prove the shipped path, then tag alpha)

## Problem

No passing run proves the actual shipped installation can deploy and operate
one real shop. Historic cells prove valuable pieces — GCP search reindexing
plus HTTP results plus post-recycle querying, 13 operator verbs, operator
rejection guards — but none is a complete store deployment, release, failed
release, post-expiry operation, backup/restore, and preview-expiry loop on the
release candidate and its artifacts. Tagging without that loop repeats the
v1 mistake at a smaller version number.

## Evidence

`intent/audit.md` F08: certified cells do not certify every production
topology; the operator run's deploy result was a rejection guard, not a
successful complete store upgrade. Name exactly what each test proves.

F04 verify half: post-expiry operation must be proved live, not assumed from
unit tests.

F10 cost half: pilot-scale spend must be recorded honestly with estimates,
missing prices, and residual-cost reporting — not a hard-cap claim.

`intent/audit.md` release-sequence rule: final release proof must reference
the actual release candidate and artifacts, not an unrelated old successful
commit. Reuse valid evidence within its scope; rerun when code, packaging, or
the public execution path changes.

## Proposed outcome

One documented GCP Autopilot recipe with explicitly selected compatible
Magento/PHP/data-service versions serves a real shop (HTTPS, assets, search,
cron/consumers where required, outbound SMTP, persistent media) through the
shipped path: initial deployment, a subsequent application release, a failed
release with diagnosis from CLI output and recovery without maintainer-only
commands, operation after credential expiry, backup and restore including
Magento's application encryption key and media, and autonomous preview expiry
with honest residual-cost reporting. Scale and combination promises stay
bounded: no arbitrary versions, no zero-downtime incompatible schema changes,
no cross-cloud DR, no hard spending cap. On green, the maintainer explicitly
tags `v0.1.0-alpha.1` (exact naming subject to the release decision) from the
proved artifacts. Pilot teams then validate time-to-working-shop,
time-to-recover, restore success, and actual monthly cost.

## Affected users and systems

Pilot agencies, maintainer release confidence. The release candidate
artifacts, the alpha recipe docs, runbooks for deploy/fail/restore/expiry,
cost reporting, evidence pack for the alpha, pilot feedback intake.

## Constraints

- Runs on the release candidate and its verified artifacts — installer,
  core, provider download — not a dev build or an old commit.
- Destroy on exit for every live session except an explicitly retained debug
  cell (no KEEP by default); per-session spend recorded.
- GCP carries live proof (existing functional evidence minimizes new spend);
  no second origin for the alpha loop.
- Service combinations and scale expectations stay bounded and written down.
- No secret values in evidence, ever.
- Human docs and website copy go through humanizer, then remove-ai-marks.
- Ask-first: the tag itself. This intent ends with a go/no-go plus the exact
  tag command; it does not push the tag.

## Out of scope

- AWS parity (owned by `aws-provider-parity` post-alpha).
- EU providers (deferred).
- New adapters, new catalog cells, importer features: gaps fail loudly and
  file back.
- Stable v1 freeze (owned by deferred `v1-stable-cut` after pilots plus AWS
  parity).

## Open questions

- Exact alpha recipe pins (Magento, PHP, MySQL, Valkey, search, RabbitMQ,
  image digests) and scale bounds (replicas, tiers, regions)? Default: the
  smallest combination the provider plus safety intents prove, written into
  the recipe. Owner: spec author with provider and safety authors.
- Pilot intake: which agency technical leads, and what feedback format
  (time-to-shop, time-to-recover, restore success, monthly cost)?
  Default: a few leads who can diagnose and report failures; structured
  intake in the spec. Owner: maintainer.
- Preview-expiry scheduler ownership and residual-cost inventory (who runs it,
  how it auths, what survives teardown)? Default: specified with the
  provider, proved live here. Owner: spec author.
