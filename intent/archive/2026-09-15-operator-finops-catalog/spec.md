# Spec: operator-finops-catalog (order 12)

## Decisions

- No `status` dashboard for v1: the verbs stay separate. A combined
  dashboard is new surface without a proven operator demand; the five
  verbs below compose in scripts already.
- The five live-proved verbs: `health --mode runtime`, `logs`,
  `exec` (plus the `--service deploy` rejection), `cost` inputs with
  live prices where offered, `cleanup reconcile` finishing an
  interrupted run.
- `pulumi-error-causes` stays archived; this intent only leans on it
  (deploy-failure classification is the other half of "what broke").
- The destroy-after-expiry trap found during order 11 (CLI allows
  expired, in-program `spec.Validate()` rejects it, all five stacks)
  is fixed here and live-proved by `cleanup reconcile`, its natural
  home.

## Live proof shape

- GCP packed session (unlimited project, destroy on exit): create one
  preview, prove all five verbs against it, interrupt one run to feed
  `cleanup reconcile`, destroy, assert clean.
- AWS without a stack: `cost --live` (Price List API, creds only) plus
  `cost --budget` read. `health`/`logs`/`exec`/`cleanup` on AWS ride
  Floci plus unit plus the Phase 2 live port exercise (mlaw1 ran the
  deploy/log paths); a second AWS stack is not worth its cost for
  verb-level proof.
- Floci plus unit for the rest: `evidence`/`audit` exports (no secret
  values), `tunnel` explicit unsupported paths, GCP budget reads,
  unpriced-list explicitness, budget-never-forecast presentation.

## Acceptance criteria

- Destroy-after-expiry fixed in all five stack packages: expired
  preview destroys via CLI, deploy of an expired preview still
  refuses. Unit tests pin both sides per provider.
- GCP session: five verb rows green, interrupted run reconciled,
  destroy plus assert clean, spend recorded.
- AWS: `cost --live` returns ECS prices with an explicit unpriced
  list; `cost --budget` reads without forecasting.
- `docs/operations.md` plus the `magelift-operate` skill describe
  exactly what was proved; docs build green.
