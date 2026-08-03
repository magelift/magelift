# ADR 0010: Database dump seed for new and ephemeral environments

- Status: Accepted (partially superseded; see Consequences)
- Date: 2026-07-21
- Superseded-by (attach scope only): Phase 8 / ATTACH-02; see [brownfield-attach.md](../brownfield-attach.md)

## Context

Agencies and small e-merchants need static and ephemeral environments seeded from
a known Magento database dump (catalog, CMS, sample B2B data). MageLift already
creates greenfield infrastructure; without a seed path, every new environment
starts empty and burns operator time.

## Decision

1. `magelift env create --dump <path>` records `environments.<name>.seedDump` as a
   local path (or later a secret-ref URI) in `magelift.yaml`.
2. The path is validated for readability at create time. Import into the managed
   MySQL/Aurora instance runs **after** the environment’s first successful
   infrastructure deploy and Magento install/upgrade candidate, not during
   Pulumi resource create.
3. AWS ECS Fargate is the first import implementation target (one-off migrate-style
   task or `magelift exec` helper). Other providers follow the same config field.
4. Until the import runner ships, the CLI records the dump and surfaces
   `seedDumpStatus` so operators know the contract without a silent no-op.

## Consequences

- Ephemeral preview envs can be created with `--dump` + TTL (`expiresAt`) and
  swept later.
- Dumps must not be committed when they contain PII; operators keep them outside
  git or use sanitized fixtures.
- ~~Brownfield “attach existing DB” remains out of scope (nice-to-have later).~~
  **Superseded (Phase 8 / ATTACH-02):** AWS RDS MySQL adopt via
  `target.aws.existing.database` is in scope; see
  [brownfield-attach.md](../brownfield-attach.md). Dump-seed decision (items 1-4
  above) is otherwise unchanged. Cloud SQL and non-AWS DB attach remain out of
  scope this milestone.

## Alternatives

- Import inside Pulumi: rejected; dumps are large, slow, and not IaC state.
- Only document manual `mysql < dump.sql`: rejected; too weak for the persona.

## Provenance

Product audit roadmap (July 2026); Chantelle / agency multi-env workflows.
