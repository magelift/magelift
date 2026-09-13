# MageLift prioritized execution backlog

Reconciled: 2026-08-23 (`certify-aws` / `certify-gcp` / `certify-ovh` / `certify-scaleway`)

Main specs: [`specs/`](specs/). How to read this tree: [`README.md`](README.md).

This file is the execution index. Requirement text lives in specs.

## Active objective

**Packed live KEEP under the per-provider certification trackers is closed.**
Catalog docs and unit proof for `certification-aws`, `certification-gcp`,
`certification-ovh`, and `certification-scaleway` are applied. Homebrew/Scoop
publish stays excluded until a public tag.

Closed live rows:

1. `certify-aws` task 3.2 — Fargate `awsba`, Managed Instances `awsmi`, and EKS Auto Mode `awsek` destroyed with `assert_clean ok`. Experimental PASS is not certified.
2. `certify-gcp` task 3.2 — `gcap29` 2.4.9 destroy emptied the prefix; 2.4.8-p5 `not-run` (no signed digest).

Campaign isolation (worktrees, unique prefixes, serial Go, isolated Pulumi
state) is in `scripts/acceptance/lib-campaign-isolation.sh`. Magento-origin
X-Ray is typed unsupported until an ObservabilityAdapter registers X-Ray.

OVH/Scaleway Magento previews stay `not-run` unless credits/`$50` cap allow a
destroy-on-exit cell. No KEEP on those providers.

Verify live work with `magelift-certify` and unique prefixes. Do not freelance
`deploy`/`destroy` outside an acceptance account.

## Do not start yet (listed only)

- Homebrew/Scoop: still blocked on a public tag. Ask before any release tag.
- Cartesian live shops. Packed KEEP only.

## Closed cloud notes (do not reuse)

Autopilot `gcap28` is the certified-row proof (destroyed). `gcap29` 2.4.9 KEEP
is closed (prefix empty). GKE Standard stays experimental. Magento 2.4.8-p5
stays `not-run` until a signed digest exists. AWS packed KEEP prefixes `awsba`,
`awsmi`, and `awsek` are empty. EKS stays experimental.

## Execution rule

Exactly one **active** objective. Mark OpenSpec tasks complete only for the
evidence class they name. Catalog docs without live Magento do not close KEEP
rows. Unit/Floci do not certify OVH or Scaleway Magento.
