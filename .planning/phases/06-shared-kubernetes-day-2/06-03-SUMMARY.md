---
phase: 06-shared-kubernetes-day-2
plan: 03
subsystem: infra
tags: [kubernetes, observe, client-go, fake-clientset, kubectl, gke, eks, ovh, scaleway]

requires:
  - phase: 06-shared-kubernetes-day-2
    provides: OutputKubeconfig + ClientFromOutputs (06-02)
provides:
  - kube.Observe implementing platform.RuntimeObserve (TailLogs/CheckRuntime/PrepareExec)
  - Four-module type-identity RuntimeObserve() → *kube.Observe
  - Portable kubectl PrepareExec (gke-job launcher debt removed)
  - CLI runExecTarget uses Launcher as binary
affects:
  - 06-04 shared kube.Steps
  - 06-05 honesty matrix / unsupported shell counts
  - Phase 7 live certification of Observe

tech-stack:
  added: []
  patterns:
    - kube.NewObserve(fake clientset) for tests; NewObserveWithFactory(ClientFromOutputs) for live modules
    - PrepareExec Args are argv after binary (AWS + kubectl share shape)
    - Fail-closed RequireStringOutput(OutputKubeconfig) in PrepareExec (T-06-08)

key-files:
  created:
    - internal/cloud/kube/observe.go
    - internal/cloud/kube/observe_test.go
    - internal/cloud/gcp/ops/observe_test.go
  modified:
    - internal/cloud/gcp/ops/day2.go
    - internal/cloud/aws/eksops/ops.go
    - internal/cloud/aws/eksops/component_test.go
    - internal/cloud/ovh/stack/ops.go
    - internal/cloud/ovh/stack/ops_test.go
    - internal/cloud/scaleway/stack/ops.go
    - internal/cloud/scaleway/stack/ops_test.go
    - internal/cli/exec.go
    - internal/cli/root.go
    - internal/cli/exec_test.go

key-decisions:
  - "Shared *kube.Observe for all four modules (D-01); no D-02 thin wrappers"
  - "PrepareExec Launcher=kubectl; require kubeconfig output; no gke-job / GKE ADC"
  - "CLI runRemoteCommand(binary, args) so kubectl and aws both work; AWS Args unchanged"
  - "CheckRuntime/PrepareExec shipped in tracer with TailLogs so GCP collapse stays complete"

patterns-established:
  - "Type-identity gate: Module.RuntimeObserve() must return *kube.Observe"
  - "fake.NewClientset for Observe unit tests — no envtest (D-06)"
  - "OVH/SCW unsupported shell shrinks when ports move to kube (Observe → 12 stubs left)"

requirements-completed: [KUBE-01, KUBE-02, KUBE-03]

coverage:
  - id: D1
    description: TailLogs via kube.Observe on fake clientset (events or clear empty-pod error)
    requirement: KUBE-01
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'ObserveTailLogs'
        status: pass
    human_judgment: false
  - id: D2
    description: CheckRuntime healthy/unhealthy from fake Deployment ready replicas
    requirement: KUBE-02
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'ObserveCheckRuntime'
        status: pass
    human_judgment: false
  - id: D3
    description: PrepareExec returns kubectl launcher + portable argv; gke-job removed
    requirement: KUBE-03
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ ./internal/cli/ -count=1 -run 'PrepareExec|ExecRunsKubectl'
        status: pass
    human_judgment: false
  - id: D4
    description: GCP/EKS/OVH/Scaleway RuntimeObserve() return *kube.Observe
    requirement: KUBE-01
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/ops/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'TypeIdentity|RuntimeObserve|OpsDeploy|ModuleAccessors'
        status: pass
    human_judgment: false

duration: 7min
completed: 2026-07-30
status: complete
---

# Phase 6 Plan 03: Shared kube.Observe Summary

**One `kube.Observe` for TailLogs/CheckRuntime/PrepareExec across GKE/EKS/OVH/Scaleway, with type-identity gates and kubectl-portable exec.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-07-30T10:54:23Z
- **Completed:** 2026-07-30T11:01:30Z
- **Tasks:** 2/2
- **Files modified:** 13

## Accomplishments

- Shipped `kube.Observe` with injected clientset / `ClientFromOutputs` factory; TailLogs/CheckRuntime use fake.NewClientset offline.
- Collapsed GCP `ops.Observe` into `kube.NewObserveWithFactory`; wired EKS/OVH/Scaleway the same way.
- PrepareExec returns `Launcher: "kubectl"` (no `gke-job`); CLI `runExecTarget` passes launcher as binary while AWS Args shape stays intact.

## Task Commits

1. **Task 1: End-to-end TailLogs via kube.Observe + fake clientset** - `f98cf3d` (feat)
2. **Task 2 RED: CheckRuntime/PrepareExec + type-identity gates** - `264bec3` (test)
3. **Task 2 GREEN: four-module wire + kubectl exec** - `829b02d` (feat)

**Plan metadata:** `edeb474` (docs: complete plan)

## Files Created/Modified

- `internal/cloud/kube/observe.go` — shared RuntimeObserve
- `internal/cloud/kube/observe_test.go` — fake clientset TailLogs/CheckRuntime/PrepareExec
- `internal/cloud/gcp/ops/day2.go` — RuntimeObserve → kube factory
- `internal/cloud/aws/eksops/ops.go` / `ovh/stack/ops.go` / `scaleway/stack/ops.go` — wire + drop Observe stubs
- `internal/cli/exec.go` / `root.go` — launcher-aware remote command runner

## Decisions Made

- D-01 type-identity over D-02 wrappers — all four modules return `*kube.Observe`.
- CheckRuntime + PrepareExec implemented with TailLogs in the tracer so GCP collapse does not leave ErrNotSupported gaps.
- CLI second `runCommand` argument is the binary name (was unused directory); default remains `aws`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Critical] CheckRuntime/PrepareExec in tracer**
- **Found during:** Task 1 (GCP Observe collapse)
- **Issue:** Collapsing GCP to `*kube.Observe` with only TailLogs would regress health/exec until Task 2.
- **Fix:** Implemented full RuntimeObserve on kube.Observe in the tracer; Task 2 TDD focused on type-identity + CLI + module wire.
- **Files modified:** `internal/cloud/kube/observe.go`
- **Committed in:** `f98cf3d`

**2. [Rule 1 - Bug] OVH/SCW unsupported method count**
- **Found during:** Task 2
- **Issue:** Removing Observe stubs broke the "15 unsupported methods" table tests.
- **Fix:** Dropped TailLogs/CheckRuntime/PrepareExec cases; count is now 12 pending 06-04/06-05 honesty pass.
- **Files modified:** `internal/cloud/ovh/stack/ops_test.go`, `internal/cloud/scaleway/stack/ops_test.go`
- **Committed in:** `264bec3`

## Threat Flags

None beyond plan register — PrepareExec still rejects via CLI NUL/newline checks; kubeconfig not logged; no GKE ADC fallback.

## Self-Check: PASSED

- FOUND: `internal/cloud/kube/observe.go`
- FOUND: `internal/cloud/kube/observe_test.go`
- FOUND: commits `f98cf3d`, `264bec3`, `829b02d`
- Verify: 43 serial tests passed across kube/gcp/eks/ovh/scaleway/cli packages
