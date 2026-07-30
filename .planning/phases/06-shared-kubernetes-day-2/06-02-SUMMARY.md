---
phase: 06-shared-kubernetes-day-2
plan: 02
subsystem: infra
tags: [kubernetes, kubeconfig, client-go, pulumi, gke, eks, ovh, scaleway]

requires:
  - phase: 06-shared-kubernetes-day-2
    provides: Phase 6 CONTEXT D-01 client-from-outputs contract (discretion RESOLVED)
provides:
  - platform.OutputKubeconfig optional Magento-facing key
  - kube.ClientFromKubeconfig / ClientFromOutputs offline factory
  - Secret kubeconfig export from GKE / EKS / OVH / Scaleway stack Outputs
affects:
  - 06-03 shared kube.Observe
  - 06-04 shared kube.Steps
  - KUBE-01..04 day-2 client wiring

tech-stack:
  added: []
  patterns:
    - Optional OutputKubeconfig (not RequiredOutputKeys) as Pulumi secret
    - ClientFromOutputs → RequireStringOutput → RESTConfigFromKubeConfig → NewForConfig

key-files:
  created:
    - internal/cloud/kube/client.go
    - internal/cloud/kube/client_test.go
    - internal/platform/outputs_kubeconfig_test.go
  modified:
    - internal/platform/outputs.go
    - internal/cloud/gcp/runtime/runtime.go
    - internal/cloud/aws/eks/runtime.go
    - internal/cloud/ovh/runtime/runtime.go
    - internal/cloud/scaleway/runtime/runtime.go
    - internal/cloud/gcp/stack/component.go
    - internal/cloud/aws/eksops/component.go
    - internal/cloud/ovh/stack/component.go
    - internal/cloud/scaleway/stack/component.go

key-decisions:
  - "OutputKubeconfig = kubeconfig; optional, not in RequiredOutputKeys (ECS free)"
  - "Plumb Runtime.Kubeconfig as Pulumi ToSecret then export via stack Outputs"
  - "EKS exports existing exec-plugin kubeconfig as-is (D-02 escape hatch deferred)"

patterns-established:
  - "Observe/Steps get kubernetes.Interface from stack outputs, never GKE ADC"
  - "Tests inject kubernetes.Interface or use BuildStaticTokenKubeconfig + valid CA PEM"

requirements-completed: []  # KUBE-01..04 listed on PLAN frontmatter as phase IDs enabled by this seam; Observe/Steps land in 06-03/06-04 — do not mark complete here

coverage:
  - id: D1
    description: OutputKubeconfig constant exists and is excluded from RequiredOutputKeys
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/platform/ -count=1 -run 'OutputKubeconfig'
        status: pass
    human_judgment: false
  - id: D2
    description: ClientFromKubeconfig / ClientFromOutputs build kubernetes.Interface offline from static-token kubeconfig; fail closed on missing/empty/invalid
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'ClientFrom|Kubeconfig'
        status: pass
    human_judgment: false
  - id: D3
    description: All four K8s stack Outputs export secret kubeconfig under OutputKubeconfig
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/stack/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'ProgramBuilds|Output|Component|Kubeconfig'
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-30
status: complete
---

# Phase 6 Plan 02: Kubeconfig Export + Client Factory Summary

**Portable `OutputKubeconfig` + `kube.ClientFrom*` factory with secret exports on all four K8s stacks — Observe/Steps no longer need GKE ADC.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-30T10:47:43Z
- **Completed:** 2026-07-30T10:52:33Z
- **Tasks:** 2/2
- **Files modified:** 12

## Accomplishments

- Added `platform.OutputKubeconfig = "kubeconfig"` as an optional Magento-facing key (not in `RequiredOutputKeys`).
- Shipped `kube.ClientFromKubeconfig` / `ClientFromOutputs` using `clientcmd.RESTConfigFromKubeConfig` + `kubernetes.NewForConfig`, fail-closed on empty/invalid input.
- Plumbed secret `Runtime.Kubeconfig` on GKE/EKS/OVH/Scaleway and exported it from each stack `Outputs()`.

## Task Commits

1. **Task 1: End-to-end kubeconfig bytes → ClientFromKubeconfig** - `4c2af70` (feat)
2. **Task 2: Export secret kubeconfig from four K8s stack Outputs** - `7adcd17` (feat)

**Plan metadata:** `148d809` (docs: complete plan)

## Files Created/Modified

- `internal/platform/outputs.go` — `OutputKubeconfig` constant + docs
- `internal/platform/outputs_kubeconfig_test.go` — optional-key / RequiredOutputKeys guard
- `internal/cloud/kube/client.go` — client factory
- `internal/cloud/kube/client_test.go` — offline static-token + fail-closed tests
- `internal/cloud/gcp/runtime/runtime.go` — `Kubeconfig` field + secret assign
- `internal/cloud/aws/eks/runtime.go` — same
- `internal/cloud/ovh/runtime/runtime.go` — same
- `internal/cloud/scaleway/runtime/runtime.go` — same
- `internal/cloud/{gcp,aws/eksops,ovh,scaleway}/stack/component.go` — Outputs export

## Decisions Made

- Key name locked as `kubeconfig` (D-01 discretion / RESEARCH Open Question 1).
- EKS keeps its exec-plugin kubeconfig export; offline client parse for AWS STS stays D-02 later if needed.
- Valid throwaway CA PEM in unit tests so `NewForConfig` can load roots without network.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] Plumb Runtime.Kubeconfig on four runtime components**
- **Found during:** Task 2 (Export secret kubeconfig)
- **Issue:** Plan listed only stack `component.go` files, but kubeconfig lived only as a local for the Kubernetes provider — stacks had no field to export.
- **Fix:** Added `Kubeconfig pulumi.StringOutput` on each runtime `Component`, assigned `pulumi.ToSecret(...)`, registered on runtime outputs, then exported via stack `Outputs`.
- **Files modified:** `internal/cloud/{gcp,aws/eks,ovh,scaleway}/runtime/runtime.go` (+ stack Outputs)
- **Verification:** ProgramBuilds mock graphs green for all four stacks
- **Committed in:** `7adcd17`

**2. [Rule 1 - Bug] Test CA must be valid PEM for NewForConfig**
- **Found during:** Task 1 (ClientFromKubeconfig)
- **Issue:** `Y2E=` (`"ca"`) fails `unable to load root certificates` inside `NewForConfig`.
- **Fix:** Use a throwaway self-signed CA base64 in `client_test.go` (never printed).
- **Files modified:** `internal/cloud/kube/client_test.go`
- **Verification:** ClientFrom* tests pass
- **Committed in:** `4c2af70`

---

**Total deviations:** 2 auto-fixed (1× Rule 1, 1× Rule 2)
**Impact on plan:** Required for correctness; no scope creep beyond exporting kubeconfig.

## Issues Encountered

None beyond the deviations above. Tracer verified offline before expansion (user-rule auto-verify of automated gate).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `ClientFromOutputs` ready for 06-03 `kube.Observe` and 06-04 `kube.Steps`.
- No cloud spend; no live cluster.
- EKS exec-plugin offline client limitations documented for D-02 if Observe wiring hits them.

## Self-Check: PASSED

- FOUND: `internal/cloud/kube/client.go`
- FOUND: `internal/platform/outputs_kubeconfig_test.go`
- FOUND: commit `4c2af70`
- FOUND: commit `7adcd17`
- FOUND: `OutputKubeconfig` not in `RequiredOutputKeys`

---
*Phase: 06-shared-kubernetes-day-2*
*Completed: 2026-07-30*
