---
status: accepted
slug: local-dev-parity
---

# Intent: local stack as close to production as Compose allows

## Problem

Bare-metal teams adopt slowly when local and production diverge: "works on my machine" becomes "fails on first deploy." `magelift local` is account-free and fast, but it is honestly not a production replica (container DB, no WAF/CDN, ephemeral TLS, different secrets). Without explicit parity work plus drift warnings, local success misleads instead of helping.

## Evidence

`docs/local-vs-cloud.md` tables what matches (PHP branch, pinned service majors, Magento CLI verbs, web runtime plugin) and honest deltas (RDS vs container MySQL, managed queue vs container broker, Secrets Manager vs `.magelift/local.env`, CloudFront/WAF/Route53/ACM absent locally). `local init` resolves the source-dated local compatibility row before writing compose; unknown release or family fails before volume creation. `local seed` installs Magento once and refuses a second install when `app/etc/env.php` exists. ADR 0006 defines generated Compose as the execution context. Real mismatch reports from shops running both paths: not checked.

## Proposed outcome

Local stays the fastest credible loop: same PHP branch and Composer contract, same service majors, same capability names, same web runtime default (nginx plus PHP-FPM) with Varnish optional, Mailpit as the explicit non-delivery sink. `local init` fails with a named reason when parity is impossible (e.g. Redis without the Adobe hatch). A drift check warns when local-only choices have no cloud equivalent. Docs state in one place what local proves and what still needs a cloud preview.

## Affected users and systems

Magento developers without cloud access, new contributors. `magelift local`, `internal/localdev`, compatibility catalog local rows, pinned images, `docs/local-vs-cloud.md`, user skill `magelift-local-runtime`.

## Constraints

No cloud credentials, Pulumi, or remote state on this path. Loopback ports and generated credentials stay local-only; never reused in deployed envs. Digest-pinned images; overrides via `MAGELIFT_LOCAL_*_IMAGE` stay deliberate. PHP settings and extension baseline verified at plan time, not reported as available metadata. Docker stays the supported host boundary.

## Out of scope

Emulating WAF/CDN/DNS, multi-AZ, managed backup semantics, or deploy migrations locally. Cheap cloud preview as iso replacement (tracked separately as a later option).

## Open questions

Which three local/cloud mismatches cause the most failed first deploys, and which become `local init` errors versus warnings? Do we add a `local doctor` style parity report for v1 or keep parity inside `local init` plus docs?
