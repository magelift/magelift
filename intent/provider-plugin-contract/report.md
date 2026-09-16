---
slug: provider-plugin-contract
verified: 2026-09-16
verdict: pass
---

# Report: provider plugin contract (the boundary v1 never had)

## What shipped

The specified boundary, as documents only — no code, no directory moves:

- `docs/adr/0013-provider-plugin-contract.md` (Accepted, 2026-09-16):
  ownership split, autonomous seven operations, versioned typed operations
  (stack lifecycle + Describe + ValidateConfig + day-2), typed errors with
  retryability, per-operation cancellation, typed progress events, opaque
  provider state, Pulumi-execution provider-side, envelope/schema split with
  resolve-time unknown-provider failure, Describe negotiation
  (protocol-major plus required operations), fail-closed failure mapping
  with explicit `--migration-mode` only, `providers/<name>/` module homes,
  `v` / `sdk/v` / `providers/<name>/v` tags with alpha lockstep plus
  specified post-alpha rules, the two follower proof gates with executable
  commands, normative no-sandbox statement, and the verbatim 0008/0011
  supersession list.
- Reciprocal scope notes on ADRs 0008 and 0011 naming standing bullets and
  0013-superseded bullets (exact-version equality, silent fallback, in-RC1
  in-process deploy, lagging extraction, in-process day-2).
- `docs/adding-a-provider.md`: contract-direction opening, `providers/<name>/`
  preamble on the checklist, rewritten plugin-load section (typed ops,
  negotiation, fail-closed, no-sandbox, current-code caveat), retired
  `Program`-returns-`any` paragraph, provider tag scheme in SDK pinning.
- `docs/architecture.md`: ownership + typed-ops + negotiation/fail-closed +
  no-sandbox boundary bullets and the specified `providers/<name>/` future.
- `contrib/skills/magelift-provider/SKILL.md`: Boundary aligned to specified
  homes and contract direction (single hunk, other sections byte-identical).

## Deviations from plan

1. Added a "Documentation updates" section to ADR 0013 so all eight spec
   requirements map to ADR sections and the box 1.1 verify reads literally.
   The section records that human docs ship with the decision; no spec
   content changed.
2. Skill alignment kept to the Boundary section per the plan's explicit
   scope: the skill description and in-process checklist still describe the
   current tree (accurate until Order 5), with the guide carrying the full
   specified direction. Full skill rewrite belongs to Order 5 (see
   Findings).
3. No contract tests or fixtures were needed: the protocol is specified as
   normative field tables per the spec's own allowance, so the "beyond"
   clause did not trigger.

## Verification

### Completeness

All 7 plan boxes ticked (1.1, 1.2, 2.1, 2.2, 2.3, 3.1, 3.2), each after its
verify clause passed. Every `### Requirement:` in `spec.md` has direct
evidence in the approved documents:

- Ownership: ADR table assigns all eight responsibility areas to exactly one
  side; seven day-2 operations plus cost-inputs listed mandatory.
- Protocol: operations list has no JSON-string or `any`-program operation;
  execution-boundary section retires the `any` shape with Pulumi
  provider-side; error codes, cancellation, and progress carry MUST
  language.
- Config split: envelope fields core-owned and core-versioned;
  `target.<provider>` blocks provider-owned with provider-published
  schemas; unknown IDs fail at resolution with the specified error
  contents.
- Negotiation: Describe fields and the major-match plus required-operations
  rule stated normatively; checksum/incompatibility/load failures each map
  fail-closed; migration requires the explicit flag with startup logging.
- Releases: `providers/<name>/` homes with one `go.mod` each; tag scheme
  plus alpha lockstep plus specified post-alpha rules; separate repos
  explicitly not required.
- Proof shapes: out-of-module `GOWORK=off` provider build and provider-SDK-free
  core `go list` gate stated as MUST-pass with commands.
- Boundary honesty: privilege inheritance plus non-sandbox status normative;
  no sandbox claim anywhere in spec, ADR, or touched human docs (grepped).
- ADR plus docs: 0013 Accepted and dated; 0008/0011 carry reciprocal notes;
  guide and architecture boundary sections read identically to the spec on
  all five points (grepped present in each file).

### Correctness

Bar is the intent's proposed outcome: an approved specification plus ADR plus
human-docs update defining the real boundary, with independent-release
mechanics specified and proof shapes followers must satisfy. All met: the
three documents agree with each other and with the traced current state they
supersede (registry, `Program(any)`, provider enum, JSON hostproto cited by
path); the supersession lists quote 0008/0011 bullets verbatim and mark them
as current-state descriptions, not rewritten history; nothing is specified
that contradicts ADR 0003/0004 topology rules. Not a UI change; observable
moments are the accepted ADR, the rebuilt docs site, and the green gates.

### Coherence

Diff follows the spec Design and repo conventions: ADR mirrors the 0008/0011
format (Status/Date, Context, Decision, Consequences, Alternatives,
Provenance); human-docs edits are minimal targeted replacements, not
rewrites; the guide keeps its checklist accurate for the current tree with
the specified direction up front; no code, schema, or directory changes.

## Findings

- SUGGESTION — The `magelift-provider` skill description and in-process
  checklist still route new-provider authors to `internal/cloud/` until
  Order 5 lands the specified move; Order 5 should rewrite the skill fully
  to the plugin model when the move lands.
  `contrib/skills/magelift-provider/SKILL.md:4`
- SUGGESTION — If Order 5 finds either proof-gate command unexecutable as
  specified, it must file back here per the spec's carried-forward question
  rather than freelancing the gate.
  `intent/provider-plugin-contract/spec.md:1`

## Not checked

- Full `go test ./...` and `make verify`: scoped to the doc test
  (`TestCapabilityMatrixDocumentsFirstPartyCatalogs`), full `go build ./...`,
  the evidence gate (still green, untouched), and `make docs` — proportionate
  for a docs-only intent; the full matrix runs in CI.
- Live CI run of this change (no PR opened from here).
- Verified in implementing session (no forked verifier; evidence is document
  diffs plus gate output above).

## Verdict

Pass. The contract is specified, decided, and documented with no code and no
moves: followers are unblocked on an explicit boundary. No CRITICAL findings.
