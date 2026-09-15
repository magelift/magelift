# Spec: eu-providers-experimental (order 11)

## Decision

Refresh infra-only live proof for both EU providers before v1. The August
2026 cells are one month old and shared Kubernetes deploy paths changed
since; "the adapter still applies" needs a current run, not a carried
claim. Magento stays `not-run`/experimental on both.

## Documented evaluation path (one shape per provider)

- Scaleway: `nl-ams` / `nl-ams-1` (`fr-par` Kapsule was in
  provider-side `shortage` on 2026-09-15 and admission correctly
  refused; same single-cell shape), one `DEV1-L` Kapsule worker, RDB
  MySQL not HA, Redis size 1, disposable `/22` VPC/private network,
  public immutable NGINX digest, `deploy --infra-only`.
- OVH: `EU-WEST-PAR`, MKS `standard` one worker one zone, managed
  MySQL, one-node Valkey discovery, disposable private network,
  public immutable NGINX digest, `deploy --infra-only`.

Both shapes already match the August cells and the experimental docs;
no shape change, only a refresh.

## Acceptance criteria

- Each provider: harness exits with `assert_clean ok`, direct
  post-run inventories return zero owned resources.
- Evidence files replaced with the September runs (same bounded
  non-claims); README rows keep pointing at the current proof.
- Capability matrix EU rows unchanged (certified subsets stay empty);
  experimental docs unchanged unless a defect moves a boundary.
- Combined live spend recorded, inside the shared $50 own-money cap
  with vendor margin intact. Serialized runs, destroy on exit, no
  KEEP, six-hour-or-shorter TTL.
- Offline gates green: provider unit/mock tests, dry-run harnesses.
