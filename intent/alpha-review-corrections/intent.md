---
status: accepted
slug: alpha-review-corrections
accepted: 2026-09-17
acceptance: review endorsed by maintainer; one correction intent
  covering R01-R12 per the review's correction sequence; archived
  reports gain dated amendments, history is not rewritten
---
# Intent: alpha review corrections (earn the archived outcomes)

## Problem

The 0.1.0 alpha review (`intent/0.1.0-alpha-review.md`, reviewed
commit `90e0dc1`) found the direction substantially better but the
completion claims ahead of the implementation: installation is not
connected end to end (R01), production GCP operations still use
expiring saved credentials (R02), media persistence and restore are
not implemented as promised (R03), deploy success overclaims
application health (R04), the provider is coupled to core internals
(R05), the provider publication test does not build a provider
(R06), the RC is not configured as a prerelease (R07), lifecycle
authority is split (R08), CI misses the nested module (R09),
mutating RPCs are misclassified retryable (R10), updater recovery
has a crash window (R11), and onboarding plus the pin sheet are
unproved (R12).

## Evidence

The review file above, verified claim by claim against source
before this intent was written. The RC run against the reviewed
commit was cancelled before publication; no release exists.

## Proposed outcome

Each finding resolved or explicitly dispositioned, in the
review's correction sequence: distribution first (R01, R07,
R11), then provider boundary plus auth (R05, R02, R10),
then shop behavior (R03, R04, R08), then CI plus publication
(R09, R06) and onboarding (R12). Archived orders 4-7 reports
gain dated correction amendments. Order 8 resumes with a
re-run loop from newly built, verified artifacts.

## Affected users and systems

Release pipeline, installer, provider loading, GCP plugin,
media path, deploy health, lifecycle, CI, onboarding docs,
and the acceptance loop inputs.

## Constraints

- One intent, explicit R-references, no new sprawling backlog.
- No tag rewrites; rc.1 stays as the reviewed candidate record.
- No bespoke acceptance-only setup; product gaps fail the phase.
- Live cloud only where a finding needs it (GCE clean-machine
  proof); destroy on exit.
- Human docs go through humanizer, then remove-ai-marks.

## Out of scope

- The acceptance re-run itself (order 8 resumes after this).
- AWS/EU/stable work (still deferred).
- New features beyond the findings.

## Open questions

None. Defaults are decided in the spec.
