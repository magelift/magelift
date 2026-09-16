---
status: specified
slug: full-deployment-coverage
intent: intent.md
---

# Spec: reference onboarding for the alpha recipe

## Requirements

### Requirement: Alpha recipe template from init

`magelift init --provider gcp` SHALL scaffold the alpha recipe: GCP
GKE Autopilot preview running Magento 2.4.9 with HTTPS, assets,
OpenSearch, DB queue, cron, persistent media, baked static content,
and a commented SMTP block plus prerequisite pointers. The starter
SHALL pass `config validate` and plan against a scripted plugin
session without edits beyond project, GCP project, and domains.

#### Scenario: Starter matches the recipe

- **WHEN** `magelift init --provider gcp` writes a starter
- **THEN** it sets provider `gcp`, runtime `gke-autopilot`, Magento
  `2.4.9`, preset `preview`, static content locales `en_US` with a
  documented theme, and no `openSearchMode: disabled` pin (omission
  defaults to the recipe OpenSearch); it carries commented SMTP and
  prerequisite pointers; `config validate` passes on the output

### Requirement: GCP SMTP wiring for operator relay

Deploy on GCP SHALL wire `email.mode: smtp` and BYO `email.mode: ses`
into Magento through Secret Manager references: host, port, username,
and sender from config, password resolved at deploy from the
`gcp-secret-manager://` credential reference into a Kubernetes Secret
consumed as Magento SMTP env. `disabled` SHALL wire nothing.
Managed `tem`/`ovh` SHALL fail closed at config validation naming
the unimplemented mode (no silent unwired email).

#### Scenario: Relay env reaches Magento

- **WHEN** a GCP preview deploys with `email.mode: smtp`, host,
  port, username, sender, and a `gcp-secret-manager://` credential
- **THEN** the workload env carries `CONFIG__DEFAULT__SYSTEM__SMTP__HOST`,
  `__PORT`, `__USERNAME`, `__AUTH=LOGIN`, `__SSL=tls`,
  `__DISABLE=0`, the sender identity, and the password from the
  referenced secret only (value never in YAML, logs, or evidence)

#### Scenario: Unimplemented managed modes fail closed

- **WHEN** config sets `email.managed` with mode `tem` or `ovh`
- **THEN** validation fails naming mode `tem`/`ovh` as not
  implemented for alpha

### Requirement: Human prerequisites named pre-deploy

`magelift doctor` SHALL guide GCP toolchain readiness (including
`gcloud` for human login flows, optional under workload identity),
and the onboarding doc SHALL name every human-action prerequisite
with a procedure before deploy: GCP billing activation, domain
ownership and DNS, Adobe licenses, SMTP relay account plus
credentials, and sandbox/sending-limit exits. F13 corrections hold:
secret references, DNS, and managed KMS do not imply manual steps
where MageLift automates them.

#### Scenario: Prerequisites visible before spend

- **WHEN** a new team reads the onboarding doc and runs `doctor`
  on the GCP starter
- **THEN** the doc lists billing, domain, license, and relay
  prerequisites with procedures, and `doctor` reports toolchain
  status with next actions instead of failing on cloud state it
  cannot see offline

### Requirement: Honest cost language end to end

Onboarding SHALL report estimates, unpriced lists, alerts, and
limits with no hard-cap or spend-stopped promise. The remaining
F10 fallout — design-system copy and the cross-provider budget
label — SHALL be fixed; `magelift cost` behavior (account-free
estimates, GCP `--live` refusal, `Enforced: false` budgets) is
verified, not changed.

#### Scenario: No cap language remains

- **WHEN** website design copy and the budget field label are reviewed
- **THEN** neither promises caps nor stopped spend, and the label
  names a planning input rather than a maximum

### Requirement: Classified deploy failures

GCP deploy and destroy failures SHALL print classified causes: plugin
typed errors map to stable exit codes with next-step guidance
(invalid input fails input-side, credential faults name
re-authentication, conflicts name the holder, upstream faults stay
operational). Raw provider text never surfaces without its class.

#### Scenario: Credential failure guides recovery

- **WHEN** a deploy fails with a provider credential error
- **THEN** the CLI exits non-zero naming re-authentication as the
  next step, and the class is asserted in tests with a scripted
  plugin session (no cloud)

### Requirement: Skills describe the built CLI

The user skills SHALL match the built CLI on the onboarding path:
rollback documents its required flags, the migrate sidecar name is
exact, and the operate skill warns that GCP `cost --live` errors.
No skill SHALL document a command, flag, or flow the CLI lacks.

#### Scenario: Skills pass their acceptance

- **WHEN** the skills suites run
- **THEN** every documented command, flag, and flow resolves
  against the built CLI, including the corrected rollback,
  sidecar, and cost-live cases

### Requirement: Security docs match the alpha runtime

The onboarding security posture (workload identity, secret
handling, network exposure) SHALL match the deployed GCP
manifests. Any F12 remainder touching the alpha recipe is fixed
in docs; runtime changes belong to their owning intents, not here.

#### Scenario: Posture claims trace to manifests

- **WHEN** each onboarding security claim is traced
- **THEN** it names the manifest or mechanism that implements it,
  or the claim is removed

### Requirement: One documented onboarding path

`docs/onboarding.md` SHALL walk install, template, prerequisites,
validate, deploy, and per-surface verification (HTTPS, assets,
search, cron/consumers, outbound SMTP, persistent media) on the
alpha recipe, linked from getting-started. Website install copy
SHALL point at the same path with honest cost and responsibility
language. Human docs and website copy go through humanizer, then
remove-ai-marks.

#### Scenario: Path walks without gaps

- **WHEN** the onboarding doc is followed step by step
- **THEN** every step names its command, expected output, and
  failure class, and no step requires an undocumented manual
  action outside the prerequisites section

## Design

Email: `sdk.Application` gains an `Email` settings struct mirroring
the cloud shape (mode, host, port, username, sender, credential
reference; no managed block — core guarantees unmanaged for GCP by
construction plus provider-side mode validation). `PlanRequest`
carries it; `PlanFromInputs` maps smtp/ses-BYО into `Spec.Email`;
the stack component resolves the password with a Pulumi
`secretmanager.GetSecretVersion` data source exactly like
`resolveEncryptionKey`, marks it secret, and mounts it into the
workload alongside `CONFIG__DEFAULT__SYSTEM__SMTP__*` env using
the same key names AWS uses. Preview default (`disabled`) is
unchanged core behavior. Managed `tem`/`ovh` rejected in
`validateCloudEmail` with `not implemented for alpha` naming the
mode. SES files untouched (complete AWS managed path, out of scope).

Config: presence of `email.managed` stays provider-routed as today;
only the tem/ovh rejection is added. `MonthlyBudgetCents` label
text becomes provider-neutral; `make generate` refreshes derived
artifacts.

CLI: `internal/cli` maps `*providerhost.PluginError` by code at
the deploy/destroy boundary (invalid→2 with input guidance,
credential→3 naming re-authentication, conflict→3 naming the
holder when present, not-found→3, upstream→1). `magelift doctor`
adds `gcloud` as guided-optional for GCP targets through the
existing toolchain spec mechanism. Init GCP starter matches the
recipe per Requirement 1 (schema-validated in tests).

Docs/skills/website: new `docs/onboarding.md` (F13-corrected
prerequisites, SMTP relay procedure with a named relay operator,
verify steps per surface reusing `magelift exec`/`health`/`logs`,
failure classes, cost honesty). Getting-started links it as the
alpha path. Website install section points at the same commands;
design MASTER copy fixed. Skill edits are surgical flag/name/note
fixes; `magelift-configure` gains no new commands.

## Gotchas / policy flags

- Secrets by reference only; the deploy-time password resolution
  returns Pulumi secrets into K8s Secrets, never into logs, plans,
  or evidence. Tests assert redaction on the SMTP env path.
- No new CLI commands: verification reuses `exec`/`health`/`logs`.
- Preview email stays `disabled` by default; the SMTP procedure
  targets staging/production-shaped environments.
- SES managed path stays AWS-only and untouched; TEM/OVH stay
  unimplemented until post-alpha demand with explicit errors.
- Preview expiry mechanics stay provider-owned; onboarding
  documents the policy text only.
- Humanizer then remove-ai-marks on all touched prose; record passes.

## Open questions carried forward

- SMTP default resolved: operator relay via `smtp` mode (ses-BYО
  rides the same mechanism). No GCP-native managed path exists;
  a managed relay would exceed alpha scope. Owner: spec author
  (decided).
- SES files resolved: keep untouched (complete AWS path).
  Owner: spec author (decided).
- Time-from-clean-workstation target: measure first in
  `reference-store-acceptance`, set the bar from pilot data.
  Owner: maintainer.
