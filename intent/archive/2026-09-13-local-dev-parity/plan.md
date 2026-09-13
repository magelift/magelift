---
status: planned
slug: local-dev-parity
spec: spec.md
---

# Plan: local stack as close to production as Compose allows

Auto-approved per the standing `/goal` instruction.

## Files that change

- EDIT `internal/localdev/hints.go`: cache product map
  (ElastiCache plus Memorystore), rabbit family modes, provisioned
  search substitute, managed-RabbitMQ naming.
- EDIT `internal/localdev/catalog.go`: drift warnings in `PlanFor`
  for disabled search and database-backed queues.
- EDIT `internal/localdev/catalog_test.go` (or new test): scenarios
  for each requirement plus the Redis regression lock.
- EDIT `docs/local-vs-cloud.md`: "what local proves" section.

## Order of work

- [x] 1.1 Cache twin plus rabbit modes plus provisioned substitute —
  verify: new unit tests pin steering and substitutes
- [x] 1.2 Drift warnings for disabled search and db queues plus Redis
  lock test — verify: unit tests assert warning text and the hatch
  error
- [x] 1.3 Docs proof-scope section — verify: `make docs` strict green
- [x] 1.4 Drive `local init` on both starters and read warnings —
  verify: AWS starter warns db queues plus disabled search; GCP
  starter warns database queues plus disabled search

## Risks

- Warning text drift between code and docs: the docs section quotes
  behavior, not strings, so wording stays free.

## Proof

Unit tests, init transcripts for both starters, docs build log.
