## 1. Preview identity contract

- [x] 1.1 Add a provider-neutral preview identity value with canonical
  repository and pull request inputs, validated environment name, Pulumi
  stack key, ownership marker, domain, branch metadata, commit digest,
  generation, and expiration.
- [x] 1.2 Add deterministic canonicalization, length, collision, and secret
  safety tests for repository slugs, pull request numbers, branch names, and
  commit digests.
- [x] 1.3 Extend resolved preview configuration and plan fingerprints with the
  identity and generation while keeping fixed staging, UAT, and production
  names unchanged.

## 2. Preview lifecycle safety

- [x] 2.1 Pass the preview identity through preview apply, redeploy, status,
  destroy, and expiration sweep operations before any provider mutation.
- [x] 2.2 Persist and verify ownership and generation metadata, refusing stale
  close or sweep mutations before provider calls and preserving actionable
  retry diagnostics.
- [x] 2.3 Add tests for two concurrent pull requests, same-PR redeploy,
  branch rename, close-after-redeploy, foreign ownership, and fixed-environment
  protection.

## 3. Provider-aware workflow generation

- [x] 3.1 Define the provider/runtime CI generator port and registry over
  resolved targets, including explicit unsupported capability errors.
- [x] 3.2 Refactor shared workflow rendering so event wiring, PR identity,
  concurrency, release pinning, and validation are provider-neutral.
- [x] 3.3 Update the AWS generator to use the PR-scoped identity contract and
  preserve existing fixed staging, UAT, and production deployment behavior.
- [x] 3.4 Implement the GCP generator with Workload Identity Federation,
  target validation, short-lived credentials, and GCP deployment commands.
- [x] 3.5 Add concurrency groups and generation inputs to preview apply and
  close jobs, with cancellation disabled for destructive lifecycle jobs.
- [x] 3.6 Add deterministic workflow snapshots and tests for GCP output,
  AWS compatibility, unsupported providers, identity isolation, and secret
  absence.

## 4. Repository integration and verification

- [x] 4.1 Regenerate example and fixture workflows and update the CLI reference
  and bootstrap guidance for PR-scoped previews and GCP federation.
- [x] 4.2 Run YAML, actionlint, and workflow-shape checks; use `act` for safe
  local jobs with provider calls stubbed or omitted.
- [x] 4.3 Run focused Go tests, `go vet` or the repository's configured static
  checks, and OpenSpec validation with strict mode.
- [x] 4.4 Record the generated workflow digest and evidence for two concurrent
  preview identities without creating paid cloud resources.
