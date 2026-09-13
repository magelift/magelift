## Context

See proposal.md for why. Constraints that shape this planning change:

- `openspec/` is gitignored as local planning. This workspace copy is the backlog until someone decides to commit it.
- Two implementation changes are still in progress and own live-evidence tasks. This change must not duplicate those checkboxes.
- The CLI tree, YAML schema, and Magento build pipeline already exist. New specs describe required behavior, including behavior that is already coded but was never an OpenSpec contract.
- Command names stay as implemented (`dev`, not `local`).

## Goals / Non-Goals

**Goals:**

- One main-spec tree an agent can read without opening ten change directories.
- Testable contracts for every in-scope product area from the completeness audit.
- One backlog file that names the single active implementation objective and the dependency order after it.

**Non-Goals:**

- Implementing remaining GCP edge, HA, DR, or AWS cells.
- Archiving the two in-progress changes.
- Un-gitignoring `openspec/` (separate decision).
- Renaming CLI commands to match the audit prompt's `magelift local` example.

## Decisions

1. **Main specs are the product truth.** Copy existing change specs into `openspec/specs/` as absolute requirements, then add this change's deltas. Archive of completed changes can happen later; it is not required to start the next implementation task.

2. **Keep existing command names.** The audit allowed this. `magelift dev` is the local contract. `doctor` is local readiness; `health` is environment evidence.

3. **Do not re-own live-evidence tasks.** GCP TLS/Armor, AWS EKS, recovery drills, and matrix closure stay in `provider-native-lifecycle-adapters` and `multi-cloud-resilience-observability-edge`. New tasks here are only for product gaps those changes do not already specify.

4. **GCP remains P0 reference; that slice is closed at the documented preview lifecycle.** The next implementation work is P1 GCP public HTTPS / Magento-origin / Armor data-plane (existing provider task 3.5). New P1 items (dump retrieve, cloud email, distribution, compliance export) start only after that active objective, or when they unblock it.

5. **FrankenPHP worker is P3.** `frankenphp-classic` stays a selectable runtime where evidenced. nginx-fpm is the primary path.

6. **Coherent capabilities, not one spec per knob.** Twelve new specs plus additive requirements on existing ones. Infrastructure building blocks land on `multi-cloud-runtime-architectures` rather than a thirteenth infra spec.

7. **Humanizer and watermarks are contributor rules**, encoded in `engineering-standards` and `skill-installation`, not shipped to end users.

Alternatives considered:

- Archive all completed changes first: useful hygiene, but would mix this audit with archive workflow and risk dropping in-progress deltas.
- Put every remaining provider task into this change's `tasks.md`: would fork ownership and go stale immediately.
- Invent `magelift local`: extra alias with no user value.

## Risks / Trade-offs

- [Risk] `openspec/` stays gitignored, so a fresh clone has no backlog → Mitigation: README states that; un-gitignore is a skippable follow-up.
- [Risk] Main specs and in-progress change specs can drift → Mitigation: BACKLOG points at change tasks as source of remaining work; new requirements go through this change's deltas.
- [Risk] Spec volume slows implementers → Mitigation: BACKLOG names one active objective; specs are reference, not a parallel todo list.
- [Trade-off] Some new requirements describe already-shipped code. That is intentional: OpenSpec was missing the contract, not always the implementation.

## Migration Plan

No runtime migration. After this change's artifacts exist:

1. Implementers read `openspec/BACKLOG.md` and take the active objective.
2. Later, archive completed OpenSpec changes so deltas fold into main specs with the usual `openspec archive` path.
3. Optionally stop gitignoring `openspec/` if the public repo should carry the backlog.

## Open Questions

None that block these specs. Whether to commit `openspec/` to the public remote is a repo-policy choice, not a product-behavior choice.
