# Governance

MageLift uses a maintainer-led, consensus-seeking model.

## Roles

- **Contributors** submit issues, documentation, code, and reviews.
- **Maintainers** review changes, manage releases and security reports, and enforce
  project policies.

Maintainers are added by consensus of existing maintainers based on sustained,
trusted contributions and sound technical judgment. A maintainer may resign or be
removed by consensus for inactivity, policy violations, or loss of trust.

## Decisions

Routine changes are decided through review. Material, irreversible, or public-contract
decisions require an ADR and a reasonable comment period. Maintainers seek consensus;
if consensus cannot be reached, the repository owner decides and records the rationale.

Security response may proceed privately and urgently. The resulting public decision
record must omit exploit details until disclosure is safe.

## Releases

Releases are cut from protected `main`, use semantic versioning, and are signed.
Automated changelogs are derived from Conventional Commits. Public v1 requires the
acceptance and name-clearance gates documented in the architecture.
