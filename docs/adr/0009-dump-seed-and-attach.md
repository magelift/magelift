# ADR 0009: Dump seed after first deploy; attach only where implemented

- Status: Accepted
- Date: 2026-08-22

## Context

New and ephemeral environments need a known Magento dump. Agencies also bring an existing AWS VPC or RDS instance. Those are different operations.

## Decision

1. `magelift env create --dump <path>` records `environments.<name>.seedDump` in `magelift.yaml`. Import runs after the environment's first successful infrastructure deploy and Magento install/upgrade, not during Pulumi create.
2. Dumps with PII stay out of git. Operators use sanitized fixtures or paths outside the repo.
3. Brownfield attach is in scope for **AWS VPC and RDS MySQL** via `target.aws.existing.network` / `target.aws.existing.database`. MageLift adopts by reference and does not take destroy ownership of the adopted VPC/RDS. There is no `magelift detach`; un-adopt is documented.
4. Cloud SQL attach and non-AWS database attach are out of scope until an adapter and evidence exist.

## Consequences

Preview envs can take `--dump` plus TTL. Greenfield (MageLift creates network and DB) remains the default.

## Alternatives considered

- Import during Pulumi create: rejected (race with Magento not installed).
- Attach as MageLift-owned destroy: rejected (would delete customer VPCs).

## Provenance

`docs/brownfield-attach.md` and `internal/cli` env seed import.
