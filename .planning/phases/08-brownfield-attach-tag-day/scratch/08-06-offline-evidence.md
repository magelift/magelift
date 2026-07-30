# 08-06 offline Floci-or-mock adopt evidence (D-04)

**Recorded:** 2026-07-30T12:24:20Z  
**Tier:** Pulumi/unit mocks (offline). Floci not run (optional; Docker/memory not required for this gate).  
**Paid AWS:** not opened in this task.

## Command

```bash
GOMAXPROCS=1 GOFLAGS=-p=1 go test \
  ./internal/cloud/aws/network/ \
  ./internal/cloud/aws/database/ \
  ./internal/cloud/aws/stack/ \
  ./internal/cli/ \
  -count=1 -run 'Existing|Adopt|Refuse|Detach'
```

**Exit code:** `0` (PASS)

## Package results

| Package | Result | Notes |
| --- | --- | --- |
| `internal/cloud/aws/network` | PASS (exit 0) | ~0.45s |
| `internal/cloud/aws/database` | PASS (exit 0) | ~0.35s |
| `internal/cloud/aws/stack` | PASS (exit 0) | ~1.22s |
| `internal/cli` | PASS (exit 0) | ~0.48s |

## ATTACH coverage (tests that ran)

### ATTACH-01 — adopt existing VPC / network

- `TestExistingNetworkUsesImportedSubnetsWithoutCreatingVPCResources` — PASS
- `TestPlanFromConfigMapsExistingNetworkInputs` — PASS
- `TestPlanFromConfigRejectsIncompleteExistingNetwork` — PASS
- `TestPlanFromConfigKeepsExistingNetworkOnly` — PASS
- `TestAdoptReportExistingNetwork` — PASS
- `TestAdoptReportEmptyWithoutExistingNetwork` — PASS
- `TestPreviewReportsAdoptNetwork` — PASS

### ATTACH-02 — adopt existing managed DB (RDS refs)

- `TestExistingDatabaseUsesRefsWithoutCreatingRDSResources` — PASS
- `TestExistingDatabaseRejectsInvalidRefsBeforeRegistration` (+ subtests) — PASS
- `TestPlanFromConfigMapsExistingDatabaseInputs` — PASS
- `TestPlanFromConfigRejectsIncompleteExistingDatabase` — PASS
- `TestPlanFromConfigRejectsWrongExistingDatabaseKind` — PASS
- `TestSpecValidateAcceptsExistingDatabase` — PASS
- `TestAdoptReportExistingDatabase` — PASS
- `TestAdoptReportNetworkAndDatabase` — PASS
- `TestNewComposesExistingDatabaseWithoutRDSCreates` — PASS
- `TestPreviewReportsAdoptDatabaseOnly` — PASS
- `TestPreviewReportsAdoptNetworkAndDatabase` — PASS

### ATTACH-03 — preview ADOPT + refuse mutate/destroy of adopted

- `TestRefuseAdoptedMutationNamesNetworkExternalID` — PASS
- `TestRefuseAdoptedMutationAllowsNonMutatingIntent` — PASS
- `TestAdoptedRefuseNamesDatabaseExternalID` — PASS
- `TestAdoptedRefuseNamesNetworkAndDatabase` — PASS
- `TestInfrastructureRefuseAdoptedNetworkMutation` — PASS
- `TestInfrastructureRefuseAdoptedDatabaseMutation` — PASS
- CLI refuse-on-existing-config helpers matched by `-run` (`TestInitFromAccRefusesExistingConfig`, `TestInitConfigOutRefusesExistingWithoutYes`, `TestDeployRefusesInfraOnlyWithoutFlag`) — PASS

### ATTACH-04 — detach / destroy does not delete adopted IDs (offline half)

- `TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs` — PASS
- Live describe-after-destroy: see `scratch/08-06-aws-adopt-confirm.md` (HUMAN_GATE / ADC)

## Floci

Not run. Optional emulator evidence only; package mocks above satisfy D-04 mocks-first. Do not treat this file as AWS certification.

## Honesty

Offline import/adopt/refuse/detach mechanics are green. Free-tier live VPC+RDS adopt confirm is a separate paid cell (spend map pass 3/3) and must not be inferred from these PASS lines.
