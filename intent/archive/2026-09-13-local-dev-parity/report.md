---
status: verified
slug: local-dev-parity
plan: plan.md
verdict: pass
---

# Report: local stack as close to production as Compose allows

## What shipped

- ElastiCache Valkey twin: AWS cache steers local to Valkey and
  records the substitute (was silently unrecorded).
- Rabbit family covers `ecs-artemis` and bare `rabbitmq` (the GCP
  default): steer plus recorded twin, provider-qualified.
- AWS `provisioned` search records its managed-domain twin.
- Drift warnings when the cloud has less than local: disabled search
  and database-backed queues each warn that local does not prove
  cloud behavior.
- Redis-without-hatch refusal locked by a regression test; docs
  gained a "what local proves" section.

## Deviations

None. Spec default taken: parity stays in `local init` plus docs,
no `local doctor` for v1.

## Evidence

- `go test ./... -count=1`: exit 0, 131 packages ok
  (`/tmp/test-local-parity.log`).
- `go test ./internal/localdev/`: 39 passed.
- Lint `internal/localdev/...`: 0 issues
  (`/tmp/lint-local-parity.log`); `go vet` clean; gofmt clean.
- `make docs`: exit 0 (`/tmp/docs-local-parity.log`).
- Transcripts: AWS staging init records Aurora MySQL, ECS RabbitMQ,
  ElastiCache Valkey twins and warns on disabled search
  (`/tmp/parity-aws/localinit.json`); GCP staging records GKE
  RabbitMQ (`/tmp/parity-gcp/localinit.json`); AWS preview warns on
  all three drift lines
  (`/tmp/parity-aws/localinit-preview.json`).

## Verdict

Pass. All six spec scenarios hold with evidence above.
