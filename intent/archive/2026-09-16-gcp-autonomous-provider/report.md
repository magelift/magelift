---
slug: gcp-autonomous-provider
verified: 2026-09-16
verdict: pass
---

# Report: autonomous GCP provider

## What shipped

GCP is an autonomous out-of-module plugin. The provider lives in
`providers/gcp/` (own go.mod), serves 30 versioned typed operations over
go-plugin net/rpc with gob, and the core talks to it only through a
plugin-backed shim plus a v2 host client. There is no embedded GCP
implementation left and no fallback path.

Surfaces and the requirements each satisfies:

- Import separation (`providers/gcp` module, zero non-test
  `internal/config` imports, shipped binary closure clean): Out-of-module
  provider build.
- `sdk/protocol.go` + `sdk/plugin.go` (30 ops, envelopes, stored plans,
  typed errors, RawExtension JSON-bytes payloads with named schemas, wire
  method table, handshake constants): Versioned typed protocol.
- `providers/gcp/plugin/` (29→30 handlers, per-op timeouts with
  mutations never retryable, credential redaction, envelope-vs-plan
  integrity checks, secret-key flags, partial-log structure): Provision
  plus full day-2 surface.
- `internal/providerhost/v2client.go` (lockfile v2 discovery, digest plus
  Cosign verify, dial, Describe negotiation of major plus all 30 ops,
  selection logging, cancel-kills-process): Explicit negotiation.
- `providers/gcp/auth/` (fresh ADC per operation, refresher with
  fail-closed expiry, raw-cause preservation, no token values in logs):
  Fresh credentials.
- Slimmed `internal/config` GCP case (presence plus generic
  at-most-one-target-block rule) with semantics in
  `providers/gcp/schema`: GCP config split.
- SDK additions (protocol messages, envelope/application/plan/tier/
  descriptor extensions, all referenced): Minimal deliberate SDK additions.
- Docs, AGENTS map, provider skill, Makefile targets, harness repoints:
  Human docs match.

## Deviations from plan

- 25→30 operations across implementation, each traced to an existing
  core surface that had to keep working: adapter proxies (purge,
  recovery), leftover backup destroy, prepare-style observe/tunnel
  methods replacing the assumed status/logs/exec, state lock/unlock/
  status, secrets list/set/remove, bootstrap verify/ensure, preview
  (automation backend parity). No speculative ops; spec op table
  evolved with the code and now matches it.
- Deploy steps run core-side from published `DeployInputsJSON` instead
  of 9 new step ops. Same guarantees, no new surface.
- Boxes absorbed across boundaries, all recorded in plan.md: synthetic
  missing-required flipped with 4.3; identity/ops/shared/cleanup
  followers landed in 4.2; 5.2 verified them green.
- Spec refinements flagged for maintainer sign-off (not freelanced
  silently): SDK-free-core gate scoped to the GCP instance until the
  last provider extracts; collector acceptance harness exempted from
  the newrelic direct-import rule by design; core keeps the 5-line
  residency backup-location leg the provider cannot see;
  capability-matrix "name the home" wording does not apply (matrix
  never named code homes; tiers verified correct instead).
- Pre-existing failures fixed along the way (all confirmed at clean
  HEAD first): pipeline staticContent fixtures, eks/ovh/scw probe
  fakes, workspace-aware lint root, zip-illegal knowledge filename.

## Verification

### Completeness

All 14 plan boxes ticked (1.1–6.2, verified by grep: 14 `[x]`,
0 `[ ]`). All 8 spec requirements have evidence below. No CRITICAL.

### Correctness

Proposed outcome (intent.md) is the bar: autonomous GCP Autopilot
provider built outside the root module, provisions plus operates over
a versioned protocol with negotiation, fails closed with no silent
fallback, fresh credentials with tested expiry/refresh/restart/CI,
no secrets in evidence, core without provider SDKs.

Human-observable moments (built `/tmp/magelift-verify` from this tree):

- `extensions list` shows `magelift-provider-gcp` provenance (mode
  reporting has a WARNING below).
- `cost --config verify-gcp.yaml --env preview` without an installed
  plugin fails closed: `open magelift.providers.lock: ...`, exit 2,
  no fallback, no panic.
- `config validate` on a structural GCP target reports `valid: true`
  (slimmed core accepts; provider validates semantics).
- Live dial test spawns the real built plugin binary, negotiates all
  30 ops, logs the selection line, and runs ValidateConfig over the
  wire (TestDialV2NegotiatesLivePlugin, 21s, PASS).

Scenario evidence:

- Separated provider build: `go build ./...` from `providers/gcp`
  exits 0; direct imports clean except the documented collector
  harness; transitive closure has no siblings, cli, or cmd.
- SDK-free core build: `make core-leanness` green (0 GCP code/SDK
  packages over `cmd/magelift`); full multi-provider form scoped
  per deviation above.
- Typed operations, no untyped blobs: gob round-trip over every
  protocol message green; adapter/output payloads are named-schema
  JSON bytes, never untyped blobs.
- Version mismatch fails closed: dispatch rejects bad versions;
  live dial with a missing op refused.
- Each operation independently exercised: 34 plugin tests with
  in-memory fakes for every op plus a real net/rpc wire round-trip;
  63 host tests including shim ports and adapters.
- Tampered plugin refuses before spawn: checksum-mismatch,
  unsigned-verify, and bundle-order tests green.
- Selected implementation always visible: selection log line
  asserted in the live dial test.
- Expiry, failure, restart, CI covered: 7 auth tests (fresh per
  call, expired refusal, raw-cause preservation, restart
  statelessness across env changes, WIF-shaped ADC construction,
  nil guards) plus server RetrieveError→credential mapping with
  re-authentication guidance; log asserts prove no token values.
- Split enforcement points: core accepts a semantically empty
  `target.gcp` (config + synthetic suites); provider ValidateConfig
  rejects it naming the field (schema suite).
- SDK stays lean: `sdk/go.mod` has zero requires; new files hold
  data types, validation, one display method, and a validated
  lookup; no interfaces, no third-party imports; every addition
  referenced by the protocol (audited per field).
- Docs describe one provider: Autopilot certified / Standard
  experimental rows verified; no page claims other extractions;
  `make docs` green.

### Coherence

The diff follows the spec Design section: one go-plugin net/rpc
surface, lockfile v2 with `magelift-v2` markers, per-op timeouts,
opaque provider state, no migration flag, dead JSON path removed
(zero references verified by grep). Patterns match the repo:
seams as constructor-injected fakes, table-driven esa tests,
fail-closed errors with tested messages.

## Findings

- WARNING — `extensions list` reported mode `in-process` when no
  plugin artifact is installed, but GCP has no in-process
  implementation left. FIXED same turn: mode is now
  `not-installed`, covered by the provenance test.
  `internal/cli/subprocess.go:110`
- WARNING — provider guide said the in-process checklist applies
  "until Order 5 lands the specified move" although the move
  landed for GCP (plus a stale "seven day-2 operations" count).
  FIXED same turn: checklist scoped to the remaining in-process
  tree with the plugin reference. `docs/adding-a-provider.md:46`
- SUGGESTION — capability-matrix requirement wording asks GCP rows
  to "name the `providers/gcp` home", but the matrix never named
  code homes; tiers verified correct instead. Consider rewording
  the spec scenario. `intent/gcp-autonomous-provider/spec.md:186`
- SUGGESTION — shellcheck warnings in acceptance scripts and the
  lint-policy 6-partition table predate this change (verified
  untouched lines / removed CI matrix); worth separate intents,
  out of scope here.

## Not checked

- Verified in implementing session (no separate verifier agent
  available in this environment).
- Live GCP: explicitly out of scope per spec (unit plus fake-client
  proof here; full-loop live proof belongs to
  `reference-store-acceptance`). No cloud spend in this intent.
- `make verify` in full (php/images/container suites): untouched
  by this change; affected suites all ran green instead.
- Release signing end-to-end (Cosign bundle issuance): owned by
  `verified-provider-distribution`; verify path unit-pinned here.
- Website copy: no website changes made, nothing to pass.

## Verdict

Pass. Both WARNINGs fixed and re-verified same turn; two
SUGGESTIONs remain as noted. Nothing left over needs its own
intent beyond the roadmap's Orders 6–8.

## Correction (2026-09-17, alpha review R02/R05/R10)

The autonomy claim outran the wiring. R02: Kubernetes clients now
default to per-request fresh ADC bearers (API, exec, tunnel) with
stale-bearer assertions and production-constructor coverage. R05:
deploy-step execution moved into the plugin behind the typed
deploy-app-phase op with SDK-governed deploy inputs; the alpha core
registers GCP plus AWS ECS only (EKS/OVH/Scaleway behind
MAGELIFT_EXPERIMENTAL_PROVIDERS); ADR 0013 reconciles the as-built
boundary with provider-import decoupling deferred to AWS parity.
R10: every operation classified by effect; mutations never retryable.
Fixed in intent/alpha-review-corrections (boxes 2.1, 2.2, 2.3).

## Correction (2026-09-18, alpha review 5.1)

The root CLI still links AWS, OVH, and Scaleway Pulumi. Import
decoupling and those extracts stay deferred to `aws-provider-parity`.
Until then, compile and test set `GOMEMLIMIT` to 75% of available RAM
instead of serializing the graph. R05 is unchanged.
