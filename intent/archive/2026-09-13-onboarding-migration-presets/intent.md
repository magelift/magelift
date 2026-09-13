---
status: done
slug: onboarding-migration-presets
---

# Intent: install, import or preset, deploy

## Problem

The target user has none to low cloud DevOps knowledge and arrives from bare metal or from ACC/Upsun. Today the path works but leaks complexity: importers emit placeholders and sidecars, presets hide provider shapes that surface later as errors, and `doctor` output does not always name the single next command. Each unclear step is a support ticket the project cannot staff.

## Evidence

`magelift init --from-acc` / `--from-upsun` generate reviewable YAML, write `<stem>.unmapped.md` for out-of-allowlist keys, exit non-zero (D-05). `docs/ece-parity.md` freezes the D-07 allowlist; free-form shell hooks and long-tail env are intentional gaps. `examples/sample-shop` covers the AWS Fargate preview; GCP/OVH/Scaleway starter coverage is thinner. `magelift doctor` resolves build plus every environment and prints readiness checks; `config validate`, `effective`, `explain` exist. Whether a representative ACC repo imports and passes `doctor` with only named residuals: not checked end to end in this session.

## Proposed outcome

A new team reaches a validated `magelift.yaml` in under 30 minutes on the local path and a cloud preview URL in about an hour after bootstrap, matching the getting-started targets. Import maps what it claims, sidecars read as a checklist with before/after for common shell hooks, presets for preview plus staging plus production exist per certified provider, and `doctor` prints the one next Magelift command. YAML stays readable and editable by hand; validation catches unsupported combinations before any mutation and names the authority (Adobe, MageLift, provider).

## Affected users and systems

New adopters, migrating ACC/Upsun shops. `magelift init`, `internal/paasimport`, presets and `internal/config` defaults, `magelift doctor`, `config` subcommands, `examples/sample-shop`, getting-started and migration docs, user skills `magelift-configure` and `magelift-migrate`.

## Constraints

Foreign PaaS YAML is never valid `--config` input; `schemaVersion: 1` probe stays. No plaintext secrets in YAML, ever; secret references only. No copied Adobe/Upsun source; clean-room importers only (`make check-clean-room`). Presets resolve to named explicit values visible in effective config; no hidden SDK defaults. Pulumi stays invisible on this path.

## Out of scope

New preset dimensions beyond what certified cells support, Cloud SQL attach, automatic websites/stores creation (those stay in Magento), dump/media cutover mechanics (covered by migration runbook and operator commands).

## Open questions

Which three starter shapes (provider plus preset plus queue plus search) do we promise and test for v1? What are the top five `doctor` failure reasons from real trial runs, and which ones become guided fixes versus docs?
