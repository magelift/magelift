## Context

See `proposal.md` for the release motivation. The repository already has `platform.StackModule`, provider-specific cloud packages, AWS and GCP acceptance scripts, checkpointed cell catalogs, and destroy plus orphan assertions. OVHcloud and Scaleway currently have mock and shared Kubernetes coverage but no live Magento acceptance harness.

Adobe's current system requirements list 2.4.6-p15, 2.4.7-p10, 2.4.8-p5, and 2.4.9 as the latest release-line entries. The catalog must follow those entries rather than treating every wire-compatible service as Adobe-certified. See the [Adobe system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements?lang=en).

## Goals / Non-Goals

**Goals:**

- Make the latest four release lines and their valid service combinations executable data.
- Reuse one cell and evidence contract across the four first-party cloud targets.
- Make cleanup a required acceptance stage, including provider-specific asynchronous deletion.
- Keep local Docker and unsupported service combinations useful as compatibility tests without inflating production certification claims.

**Non-Goals:**

- Certifying every historical patch version.
- Claiming Adobe support for combinations Adobe marks unsupported.
- Making local Docker equivalent to a managed cloud target.
- Adding Fastly lifecycle behavior in this change. That belongs to `fastly-edge-and-paas-migrations`.

## Decisions

### 1. Use a source catalog with generated validation and documentation

The release-line and service records will live in a reviewed machine-readable catalog under the compatibility package. Go validation will consume the typed catalog, and documentation or matrix views will be generated from the same records. This avoids duplicating version rules in YAML schema, provider code, and hand-maintained tables.

Alternative rejected: keeping the matrix only in Markdown. It cannot reliably drive pre-mutate validation or cell generation.

### 2. Keep Adobe support separate from MageLift compatibility

The catalog will not turn a passing integration test into an Adobe certification. A cell can be `mageLift-compatible` for a provider-specific or wire-compatible service while remaining outside the `adobe-supported` release gate. `allowUnsupported` remains an explicit escape hatch, not a certification mechanism.

Alternative rejected: testing every requested combination and calling all green results certified. Adobe's version-specific tables already exclude combinations such as Redis on the latest 2.4.6 and 2.4.7 rows.

### 3. Extend the existing acceptance shape instead of building a second runner

AWS and GCP scripts already implement create-once, checkpoint, cell updates, destroy, and cleanup assertions. The change will extract the shared cell and evidence contract while retaining provider-specific lifecycle functions. OVHcloud and Scaleway will add adapters for their managed Kubernetes resources and their provider APIs.

The shared cell consumes the portable architecture's independent native and
external edge/observability intents. It must not reintroduce a singular
`edge.provider` or `observability.provider` selector; target and resource
`provider` values remain identity fields only.

Alternative rejected: one provider-neutral resource deleter. Provider APIs and asynchronous deletion behavior differ too much, and a broad deleter would risk unrelated user resources.

### 4. Use exact ownership markers for cleanup

Every run will receive a unique prefix and a provider-specific ownership marker. Cleanup will use those markers plus stack state. It will never delete all resources of a service type or an entire account/project. Existing resources referenced by configuration remain outside the cleanup scope.

### 5. Treat required cells as release data

The catalog will mark cells as required, optional, unsupported, or unavailable. The RC1 gate will fail when a required cell is missing evidence, fails, or cannot run. An unavailable provider product must be fixed, removed from the required set with an explicit scope decision, or kept outside the RC1 claim.

## Risks / Trade-offs

- [Adobe updates the system requirements] -> Store source revision and retrieval date, regenerate the catalog, and invalidate affected evidence.
- [The matrix becomes too expensive] -> Reuse stacks for mutable cell updates, use preview and mocks for unsupported or costly paths, and reserve live apply for catalog-required cells.
- [Provider cleanup misses an asynchronous resource] -> Add bounded polling, a second orphan scan, and a recorded failure rather than declaring success.
- [Adobe Commerce credentials are unavailable] -> Keep edition in the cell identity and publish no Adobe Commerce certification row until a licensed artifact and composer credentials are supplied.
- [A provider has no equivalent service] -> Record `unavailable`; do not substitute a different service without an explicit compatibility classification.

## Migration Plan

1. Add the catalog and validation tests without changing current certified labels.
2. Convert AWS and GCP evidence to the shared cell format and rerun their offline harnesses.
3. Add live OVHcloud and Scaleway acceptance adapters behind experimental labels.
4. Run the required cells with unique prefixes and record generated evidence.
5. Update capability and release documents only from passing evidence.
6. Remove temporary credentials and verify all four accounts after the final cleanup scan.

Rollback is deleting the new catalog and harness wiring while retaining existing acceptance scripts. No live resource should be retained as part of the migration.
