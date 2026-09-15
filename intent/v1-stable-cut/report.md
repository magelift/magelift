# Report: v1-stable-cut tag half (order 14)

The tag, README flip, and install-doc flip stay the user's release
call. This report covers gate re-verification only. No tag created.

## Prior cycle

Order 7 (engineering half) passed in history: Windows zip upgrade,
provider skew warning, smoke asserts both binaries, `cask-verify`
job, install-asset audit with no drift. Its two notes for this
cycle are both closed below.

## What this cycle did

- Fresh `make license-check` exit 0 (2026-09-15) and fresh `make
  release-smoke` exit 0, both binaries, ~3m18s. Gate board re-dated.
- Release pipeline inspection: GoReleaser archives plus
  `checksums.txt`, SBOM (syft), keyless Cosign signs with
  `verify-blob` against the workflow identity, GitHub artifact
  attestations, `cask-verify` on macos-latest (tap, install,
  `magelift version`), container SBOM plus SLSA `mode=max` plus
  Cosign sign/verify. All present in definition; all but
  `cask-verify` proven by the dialproof tag runs.
- Upgrade verify path: 6 tests green (checksum verify plus
  mismatch rejection, unsigned rejection, invalid-signature
  rejection, Windows zip).
- Phantom-tag reword: `release.yml` and `images.yml` cited
  `v1.0.0-rc.1` as past observation though no such tag exists.
  Reworded to state the constraint without the false witness
  (closes the order-7 note).
- Homebrew decision: cask ships with rc.1 per the board; its first
  green run is part of the tag proof (closes the order-7 note).
- Install flip readiness: `install.md` already carries the full
  archive-first section behind a note admonition; README needs a
  two-line flip. Verified ready, not flipped.

## CI triage (run 35021580591 and predecessors)

- gofmt drift in `localdev/catalog.go` (prior) and `platform/env.go`
  (order-8 https fix): fixed, formatting-only.
- `run-harness-step.sh` passed `--` to GNU `timeout`, which rejects
  it (uutils accepts): fixed, verified on both flavors.
- FrankenPHP gobinary 5 HIGH: fixed versions exist upstream but
  v1.12.7 is still the latest release, so dated `.trivyignore`
  entries with a re-check trigger (RC tags publish no images).
- `gcp cleanup provider` default wiring plus native-search `https://`
  prefix (both order-12, live-proved) broke two stale test
  expectations: tests updated to the proved behavior.
- `acceptance contracts` cold-cache `evidence_seal` build outgrew
  its 900s ceiling, then a 1500s ceiling: root cause was the
  Makefile-serial `GOMAXPROCS=1 -p=1` compile of `./cmd/magelift`
  (single-threaded cold builds exceed 25 min on CI runners).
  Fixed with a bounded parallel build inside the test
  (`nproc` procs, 4GiB), module-cache restore kept, ceilings
  restored to 900s / 25 min.
- `shellcheck`: 46 warnings, all introduced on `wip/all-local-work`
  (`main` is clean), none in files this session touched. Acceptance
  scripts do not ship in CLI archives. Fixing 46 warnings across
  live harness scripts is a dedicated cleanup, explicitly out of
  this order. PENDING FINAL RUN.

## CI verdict

PENDING: run 35030388789.

## Go / no-go per tag gate

PENDING CI verdict. Working state: license go, smoke go, pipeline
go, upgrade go, docs go, contracts PENDING, shellcheck known-red
(out of release scope, needs its own cleanup).

## Tag command (DO NOT RUN without the user)

```sh
git tag -a v1.0.0-rc.1 7b1a618 -m "MageLift v1.0.0-rc.1"
git push origin v1.0.0-rc.1
```

Then: flip README plus `install.md` to archives-first, watch
`release.yml` (archives, SBOM, signs, attest, `cask-verify`) go
green on the tag. Note the tag lands on `wip/all-local-work`, not
`main`; say so in the release notes or merge first.

## Spend

CI minutes only. No cloud spend in this order.
