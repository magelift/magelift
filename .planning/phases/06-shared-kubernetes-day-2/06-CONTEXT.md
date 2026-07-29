# Phase 6: Shared Kubernetes Day-2 - Context

**Gathered:** 2026-07-29
**Status:** Ready for planning

<domain>
## Phase Boundary

One Kubernetes day-2 implementation in `internal/cloud/kube` serves GKE Autopilot, EKS Autopilot, OVH MKS, and Scaleway Kapsule: shared `Observe` (TailLogs/CheckRuntime/PrepareExec) and shared `deploy.Steps`, plus OVH/Scaleway S3-compatible state via endpoint override. Offline only (fake clientset / envtest / local S3 endpoint). No live GCP (Phase 7). Bootstrap/Secrets/Cost stay per-provider with loud tier-named failures.

</domain>

<decisions>
## Implementation Decisions

### Shared Observe
- **D-01:** Ship concrete `kube.Observe` implementing `platform.RuntimeObserve`. All four Kubernetes modules’ `RuntimeObserve()` return that shared type (ctor OK). Client from stack kubeconfig/outputs; fake clientset inject for tests. Collapse GCP’s provider-local Observe into `kube`; do not expand `platform` with kube-specific methods; do not copy ECS Observe into each provider. — **Reversibility:** one-way — type-identity tests become the contract
- **D-02:** Thin per-provider Observe wrappers only as an escape hatch if a provider cannot emit usable kubeconfig — not the default. — **Reversibility:** reversible

### Shared deploy.Steps
- **D-03:** Single `kube.Steps` in `internal/cloud/kube` driving Validate → RegisterCandidate → RunMigrations → CleanupCandidate → UpdateServices → Stabilize → Health → Record. Each provider’s `NewDeploySteps` returns/wraps that type. Do not compose four provider Steps types; do not create `internal/deploy/kube`; do not merge ECS deployflow into kube. — **Reversibility:** one-way — KUBE-04 type-identity gate

### OVH / Scaleway state
- **D-04:** One shared S3-compatible state manager; OVH/SCW pass endpoint + creds (+ SSE policy generalization so AWS keeps KMS while others use compatible encryption). Prove offline against local S3-compatible endpoint. No forked state packages; no “Pulumi DIY only” cut for MageLift lock/backup. — **Reversibility:** costly — AcquireLock hard-fail flip depends on this seam

### Unsupported / honesty
- **D-05:** Where shared kube provides Observe/Steps (and State after D-04), delete stub methods and return the shared concrete types. Keep thin `unsupported{}` only for Bootstrap/Secrets/Cost (and any remaining cloud-shaped gaps) returning `platform.ErrNotSupported` with certification tier named by CLI — never `nil` success, never panic. Matrix-record working vs gap cells. — **Reversibility:** costly — TRUST-02 / capability-matrix honesty

### Phase spend / proof
- **D-06:** Phase 6 is zero cloud spend. Live GKE exercise is Phase 7 only. — **Reversibility:** N/A (spend map)

### the agent's Discretion
- Exact client factory / kubeconfig output key names (keep portable across four targets)
- Whether envtest is required vs fake clientset alone for Steps sequence
- SSE generalization API shape inside the existing AWS state package
- Matrix cell wording for remaining Bootstrap/Secrets gaps on experimental OVH/SCW

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase / requirements
- `.planning/ROADMAP.md` — Phase 6 goal + SC1–SC5 + spend map
- `.planning/REQUIREMENTS.md` — KUBE-01..07
- ADR 0002 / 0004 / 0008 / 0009 — platform ports vs cloud/kube placement

### Existing code
- `internal/cloud/kube/` — helpers today; home for Observe + Steps
- `internal/platform/observe.go` — `RuntimeObserve` port
- `internal/cloud/gcp/ops/day2.go` — provider Observe to collapse
- `internal/cloud/aws/eksops/ops.go`, `ovh/stack/ops.go`, `scaleway/stack/ops.go` — `unsupported{}` shells
- `internal/cli/ports.go` — tier naming for `ErrNotSupported`
- AWS state manager + Floci / `NewAWSWithEndpoint` — S3-compatible endpoint path

### Deferred
- Phase 7 — live GKE Autopilot certification + MIGRATE-04 DNS HUMAN_GATE + managed dump cell
- Phase 8 — brownfield attach

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `platform.RuntimeObserve` + CLI logs/exec/health wiring
- `internal/cloud/kube` helpers (env vars, kubeconfig builder, skip-await annotations)
- AWS ECS Observe/Steps as pattern reference (not to merge)
- S3 state manager with endpoint override for Floci

### Established Patterns
- Port-nature split: k8s-shaped → `internal/cloud/kube`; cloud-shaped → per-provider
- Loud `ErrNotSupported` + tier in CLI (TRUST-02)
- Offline proof before paid passes

### Integration Points
- Four modules: gcp, aws/eksops, ovh, scaleway register shared Observe/Steps
- Capability matrix + release-readiness rows updated for honesty

</code_context>

<specifics>
## Specific Ideas

- [--auto] Persona synthesis (Staff Platform / DX CLI / Agency Lead / Release Manager / Magento Cloud Operator) locked D-01..D-06
- Type-identity tests: all four modules return the same `kube.Observe` / `kube.Steps` concrete types
- GCP provider-local Observe must die or become a one-liner into kube

</specifics>

<deferred>
## Deferred Ideas

- Live GKE Autopilot logs/exec/deploy/health — Phase 7
- Full OVH/SCW Bootstrap/Secrets implementations — intentional gap until product prioritizes
- Merging ECS and kube deploy into one mega Steps — rejected

</deferred>
