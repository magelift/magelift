---
phase: 08-brownfield-attach-tag-day
reviewed: 2026-08-03T11:11:26Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/automation/pulumi.go
  - internal/automation/pulumi_outputs_test.go
  - internal/cloud/kube/observe.go
  - internal/cloud/kube/observe_test.go
  - internal/cli/env.go
  - internal/cli/lifecycle.go
  - internal/cli/logs.go
  - internal/cloud/aws/stack/spec.go
  - internal/cloud/aws/database/database.go
  - internal/cloud/aws/stack/spec_test.go
  - scripts/gcp-acceptance-local.sh
findings:
  critical: 2
  warning: 5
  info: 0
  total: 7
status: issues_found
---

# Phase 8: Code Review Report

**Reviewed:** 2026-08-03T11:11:26Z
**Depth:** standard (Bugbot-style; uncommitted production delta)
**Files Reviewed:** 13
**Status:** issues_found

## Summary

Uncommitted delta covers GCP Composer secretref correction, decrypted Pulumi outputs + CLI redaction, kube BindOutputs/PrepareExec kubeconfig wiring, dumpimport env overrides, RDS `!` ARN allowlist, and GCP acceptance harness fixes. Directionally correct (inverted GCP validator fix and Outputs/RedactedOutputs split are sound). Two high-severity secret-handling gaps must be fixed before commit; several medium robustness/evidence issues should be addressed or explicitly accepted.

## Critical Issues

### CR-01: DB password lands on local `kubectl` argv via new dumpimport env wiring

**File:** `internal/cli/env.go:672-674` (flows into `internal/dumpimport/runner_kube.go:110-114`)
**Issue:** `MAGELIFT_DUMPIMPORT_PASSWORD` is newly accepted by production CLI and passed as `opts.Password`. The kube runner embeds it in kubectl argv as `env MYSQL_PWD=<password> mysql …`. That exposes the Cloud SQL password in the local process table (and often kube/API audit of the exec command). Harness `scripts/gcp-acceptance-local.sh:851-857` actively exercises this path. T-07-08 only avoided `-pPASSWORD`; host argv still leaks the secret.
**Fix:** Do not put the password on kubectl argv. Prefer one of:
```go
// Option A: pass MYSQL_PWD via DefaultKubeExec env to a wrapper that reads env
// inside the pod without echoing it on argv — e.g. create the mysql client pod
// with envFrom a short-lived Secret, then exec without MYSQL_PWD= in args.
args = append(args, "mysql", "-h", k.host, /* no MYSQL_PWD on argv */)
```
Or pipe a here-doc/`mysql --defaults-extra-file=/dev/stdin` over kubectl stdin. Gate `MAGELIFT_DUMPIMPORT_*` behind an explicit acceptance-only flag if this channel must remain for the harness.

### CR-02: `PrepareExec` writes kubeconfig to `/tmp` and never deletes it

**File:** `internal/cloud/kube/observe.go:207-249` (caller: `internal/cli/exec.go:104-117`)
**Issue:** `writeKubeconfigTemp` persists a decrypted cluster kubeconfig under `/tmp/magelift-kubeconfig-*.yaml` (mode 0600) and returns the path in `ExecTarget.Args`. `runExecTarget` never removes the file on success or failure. Every `magelift exec` / acceptance `day2:exec` leaves long-lived cluster credentials on disk; path is also visible in process argv via `--kubeconfig`.
**Fix:**
```go
func (o *options) runExecTarget(ctx context.Context, target platform.ExecTarget) error {
	defer cleanupKubeconfigArgs(target.Args) // remove --kubeconfig <path> temp files
	// ... existing run ...
}

// In PrepareExec / writeKubeconfigTemp: document ownership, or return a cleanup func
// alongside ExecTarget and invoke it from CLI after CommandContext returns.
```
Also consider `os.CreateTemp` in a private dir under `os.UserCacheDir()` with explicit remove-on-exit.

## Warnings

### WR-01: `outputs` CLI fail-open prints decrypted secrets if redaction errors

**File:** `internal/cli/lifecycle.go:285-296`
**Issue:** Command loads decrypted `Outputs`, then replaces with `RedactedOutputs` only when `redErr == nil`. Any redaction failure silently falls through to `o.write` of full secrets (kubeconfig, DB passwords) — often tee'd into acceptance logs.
**Fix:**
```go
if redactor, ok := backend.(interface {
	RedactedOutputs(context.Context) (map[string]any, error)
}); ok {
	outputs, err = redactor.RedactedOutputs(cmd.Context())
	if err != nil {
		return fmt.Errorf("read redacted infrastructure outputs: %w", err)
	}
} else {
	outputs, err = backend.Outputs(cmd.Context())
	if err != nil {
		return fmt.Errorf("read infrastructure outputs: %w", err)
	}
}
```

### WR-02: Removing `-t` from kubectl exec breaks interactive TTY sessions

**File:** `internal/cloud/kube/observe.go:216-218`
**Issue:** Args changed from `-it` to `-i` so scripted acceptance works without a TTY. Interactive `magelift exec -- bash` (or similar) will no longer allocate a TTY — broken prompts/job control for operators.
**Fix:** Allocate `-t` only when stdout is a terminal:
```go
args := []string{"--kubeconfig", kubePath, "exec", "-n", o.ns(), "-i"}
if isTerminal(os.Stdout) {
	args = append(args, "-t")
}
args = append(args, "deploy/"+deploy, "--")
```

### WR-03: Harness forges seed-dump journal `status=recorded` outside ADR 0010 path

**File:** `scripts/gcp-acceptance-local.sh:820-830`
**Issue:** Cell `migrate:dump` writes `.magelift/seed-dumps/<env>.json` via inline Python as `"status": "recorded"` to bypass `env create --dump`. That skips the real journal lock/InitRecorded contract and can certify a cell that never recorded operator dump intent through the product API.
**Fix:** Call `magelift`/`seeddump.InitRecorded` (or a supported CLI) to mark recorded; only forge in dry-run shape tests, not live certify cells.

### WR-04: `preview_reports_stale_state` fail-open can miss dirty Pulumi state

**File:** `scripts/gcp-acceptance-local.sh:407-424`
**Issue:** Preview failure / missing jq / parse errors now return clean (`return 1`) instead of stale. In `reconcile_stale_pulumi_state`, a false “clean” preview after export checks can let reconcile succeed while stack metadata is still wrong — next create-once may collide or skip needed `stack rm`.
**Fix:** Keep export-based `pulumi_stack_has_managed_resources` as primary, but treat preview transport/parse failure as **inconclusive** (fail reconcile / force `stack rm`) rather than “no stale proof.”

### WR-05: Acceptance writes decrypted kubeconfig into `LOG_DIR` without cleanup

**File:** `scripts/gcp-acceptance-local.sh:699-702`, `832-837`
**Issue:** `pulumi stack output kubeconfig --show-secrets` is written to `${LOG_DIR}/cutover-dns.kubeconfig` and `migrate-dump.kubeconfig` (chmod 600). Files persist under the workdir logs tree after the cell; easy to archive/commit accidentally with evidence bundles.
**Fix:** `trap 'rm -f "$kc_path"' RETURN` (or EXIT) after each use; prefer process substitution / temp dir under `mktemp -d` that the EXIT trap removes.

## Info

None (style nits omitted per instructions).

## Notes (no defect / verified OK)

- `internal/config/config.go:401-404` — inverted GCP Composer secretref check is correctly fixed (`!= GCPSecretManager` rejects wrong schemes); tests match.
- `internal/automation/pulumi.go` — decrypted `Outputs` for day-2 + `RedactedOutputs` for display is the right split; unit coverage in `pulumi_outputs_test.go` is appropriate.
- `internal/cli/logs.go` — `BindOutputs` before `TailLogs` correctly closes the factory-only Observe gap.
- RDS secret ARN `!` allowlist in `spec.go` / `database.go` matches ManageMasterUserPassword (`rds!db-…`); test covers a real-shaped ARN.
- Docs certification flips were not treated as code defects; honesty depends on matrix evidence already cited in those docs.

---

_Reviewed: 2026-08-03T11:11:26Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
