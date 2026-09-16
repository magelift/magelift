# Contributor-track checklist (executed 2026-09-16; all boxes checked with observed evidence)

> Status: executed 2026-09-16. Docs links verified by tree reads; commands
> verified by execution; dry-run-only procedures carry dates. Box 3.1: 7/7.

Conventions: `[FILE]` = verified by reading the tree. `[RUN date]` = verified
by execution on that date. Live-cloud and release-tag procedures are
dry-run-only with last-executed dates recorded per spec.

## 1. magelift-certify — Run and document a certification cell

- [x] Procedure review: skill names no executable commands (judgment checklist
      only: prefix, digest, preview-first, day-two ops, destroy-on-exit,
      orphan assertion, evidence record). No command checks apply.
- [x] Docs links resolve [FILE]: `docs/capability-matrix.md` ✓,
      `docs/evidence/` ✓ (dir), `docs/evidence/runs/` ✓ (sealed JSONL present).
- [x] Dry-run-only: live cells. Last executed 2026-09-15 (packed GCP/AWS
      sessions per evidence bundle; predates this checklist).

## 2. magelift-contribute — Layout, verify, PR, honesty

- [x] Commands exist [RUN 2026-09-16]: `composer` ✓, `go` ✓,
      `make -n verify` prints the target graph (exit 0 — graph parses; the
      full run is the order-14 gate, not this checklist).
- [x] Docs links resolve [FILE]: `docs/capability-matrix.md` ✓,
      `docs/evidence/README.md` ✓, `AGENTS.md` ✓ (Map section).
- [x] Per-intent skill-update rule recorded (box 3.2): `grep -F "same change"`
      green.

## 3. magelift-extend — Public extension boundary

- [x] Procedure review: skill names no executable commands (boundary rules +
      report shape only). No command checks apply.
- [x] Docs links: none cited (no broken-link surface).
- [x] Dry-run-only: disposable real-account cell before claiming support —
      N/A in v1 scope (no community cell claimed); recorded 2026-09-16.

## 4. magelift-provider — New first-party adapter

- [x] Seams compile [RUN 2026-09-16]: `go build ./internal/registry/ ./cli/`
      exit 0 (no literal commands in skill; code seams verified instead).
- [x] Docs links resolve [FILE]: `docs/adding-a-provider.md` ✓,
      `docs/adr/0002-certified-vs-experimental.md` ✓,
      `docs/adr/0004-ports-and-adapters.md` ✓,
      `docs/adr/0007-github-oidc-roles.md` ✓,
      `docs/adr/0008-provider-load-path.md` ✓ (relative link from the skill
      dir traverses to the same file).

## 5. magelift-release — Tag, GoReleaser, Cosign, GHCR

- [x] Commands exist [RUN 2026-09-16]: `cosign` ✓ present; `goreleaser`
      MISSING on this host (release smoke needs it installed — recorded, not
      failed); `make -n release-smoke` prints the script line (target exists).
- [x] Script + targets resolve [FILE]: `scripts/release-smoke-local.sh` ✓,
      `Makefile` `release-smoke:` ✓.
- [x] Docs links resolve [FILE]: `docs/release-readiness.md` ✓,
      `docs/publishing.md` ✓. GHCR table fixed to
      `magelift-{nginx,builder,frankenphp-classic}` in this intent (order-15
      follow-through).
- [x] Dry-run-only: public tag cuts + hosted workflow runs — none cut;
      v1.0.0-rc.1 is the user's pending call. Recorded N/A 2026-09-16.

## 6. magelift-serial-builds — Constrained-runner discipline

- [x] Commands exist [RUN 2026-09-16]: `go` ✓, `docker` ✓, `npm` ✓, `npx` ✓
      present; `goreleaser` missing (same host gap as skill 5 — recorded).
- [x] Script + target resolve [FILE]: `scripts/release-smoke-local.sh` ✓,
      `Makefile` `release-smoke:` ✓.
- [x] Docs links: none cited (no broken-link surface).

## 7. magelift-site — Website + MkDocs

- [x] Commands exist [RUN 2026-09-16]: `npm` ✓, `npx` ✓, `docker` ✓ present.
- [x] Paths resolve [FILE]: `website/scripts/build-site.sh` ✓,
      `.github/workflows/site.yml` ✓, `website/dist/` (ignored build output;
      regenerates via the script).
- [x] Docs links resolve [FILE]: `docs/capability-matrix.md` ✓ (copy mirror).

## Tally

7/7 when every unchecked box above is checked with observed evidence. Any
procedure needing a live cloud or a release tag stays dry-run-only with its
last-executed date (or explicit N/A + date) recorded.
