## Context

See proposal.md for why. Constraints: no public users, so CLI and schema can change in place. Pulumi Automation API already runs under Magelift; `doctor` currently lists `pulumi` as required. `application.webRuntime` currently enumerates FrankenPHP. Local Compose is `magelift dev`. Magento runtime overlays do not exist in `internal/config/model.go`. Quality Patch IDs are read from `.magento.env.yaml`. OpenSpec lives in a gitignored `openspec/` tree.

## Goals / Non-Goals

**Goals:**

- Decide how YAML, CLI verbs, and local Compose stay one product for Magento PHP teams.
- Decide nginx-only schema, `local` rename, overlay field shape, and SQS/Pub/Sub as a module integration.
- Order implementation so local iso-prod and runtime config land before dense experimental cells.

**Non-Goals:**

- Certifying ECS Managed Instances, Aurora, OpenSearch Magento data-plane, or RabbitMQ quorum in this change.
- Wiring Magento deploy through go-plugin Dial.
- Moving contributor skills into `.agents/skills/`.
- Application code in this OpenSpec change.

## Decisions

1. **`magelift local` replaces `dev` in the Cobra tree.** Magento developer mode is a different concept. Rename commands, tests, skills, and generated CLI reference together. No alias.

2. **nginx-fpm is the only `application.webRuntime` enum value.** Drop FrankenPHP and Apache from schema generation. Unknown values already fail closed.

3. **Magento overlays live under portable YAML, not raw `env.php` files in git.** Secret references only. Import maps ACC/Upsun and `.magento.env.yaml` patch IDs into `magelift.yaml`. Build reads Magelift YAML only.

4. **Local Compose is iso-prod of the selected environment, not a second catalog.** Named substitutes (Aurora→MySQL same major, and the rest) recorded in provenance. Split cache/session when the cloud preset is durable. Do not emulate CloudFront/Armor/Fastly.

5. **SQS/Pub/Sub are `integrations` plus a locked Composer Magento module, not `queueMode`.** Core Magento consumers stay db/AMQP.

6. **`audit` is a control matrix; `evidence` stays the change journal.** Prefer `magelift audit` as its own command if it stays small; otherwise `evidence --controls`.

7. **BACKLOG may name a non-provisioning active objective** when no Magento cell is live. First implementation slice: `local` + runtime config + integrated storefront example.

8. **Adobe-unsupported fails closed; MageLift-experimental warns and proceeds.** `compatibility.allowUnsupported` is the Adobe hatch only. Uncertified-but-implemented cells (EKS, OVH Magento, dense AWS profiles) MUST warn, record experimental, and MUST NOT block. Provider-unavailable (no adapter/SKU) still fails. Missing Magento-module SQS still fails.

Alternatives considered:

- Keep `dev` as alias: rejected; no users, Magento-mode collision.
- Warn-and-proceed Adobe: rejected; fail-closed + `allowUnsupported`.
- Fail-closed MageLift-experimental: rejected; warn-only, non-blocking, never silent, never certified.
- `queueMode: aws-sqs`: rejected; not Magento AMQP.

## Risks / Trade-offs

- [Risk] Overlays become a second Magento config language → Mitigation: map ACC `CONFIG__*` and env.php keys; reject unknown core keys; secret refs only.
- [Risk] Iso-prod local still cannot be Aurora or CloudFront → Mitigation: named substitutes and doctor cloud-only notes, never silent.
- [Risk] Spec volume vs implementation → Mitigation: BACKLOG one slice; P2 cells stay experimental in the catalog.
- [Risk] Experimental warn-only lets operators apply uncertified cells → Mitigation: warning names MageLift, provenance records experimental, certification rows stay closed.
- [Trade-off] Removing FrankenPHP drops an evidenced experimental runtime. Accepted: Adobe nginx only, unreleased product.

## Migration Plan

No public migrate. Contributors: update YAML fixtures, Cobra, schema via `make generate`, user skills, docs. Rollback is git revert of that implementation PR.

## Open Questions

None that block these specs. Field names for Magento overlays (`application.runtime` vs `magento.env`) can be chosen in the implementation PR as long as they remain portable YAML with provenance.
