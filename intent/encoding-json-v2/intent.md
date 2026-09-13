---
status: draft
slug: encoding-json-v2
---

# Intent: use encoding/json/v2 at Magelift JSON boundaries

## Problem

Go 1.27 already backs `encoding/json` with json/v2 internally. Magelift still calls the v1 API at CLI `--output json`, the build-runner protocol, and other public JSON. We did not migrate to the v2 API during the bump. Error text and unknown-field behaviour can already drift under the v1 names. Taking the v2 API would let Magelift set that behaviour on purpose instead of inheriting defaults.

## Evidence

`go.mod` is `go 1.27.0` / toolchain `go1.27.1`. Many packages import `encoding/json` (`internal/cli`, `internal/config`, `internal/build/runner`, providerhost). Focused tests after the bump passed without `GOEXPERIMENT=nojsonv2`. Byte-for-byte stability of CLI JSON vs 1.26: not checked. How many call sites need v2 options (unknown fields, case, time): not checked.

## Proposed outcome

Public JSON Magelift prints or accepts is produced through `encoding/json/v2` (or a thin helper) with explicit options. Tests lock the contract (unknown fields, error strings) so a later Go default change does not silently change `--output json`. Operators see the same fields they see today unless a spec says otherwise.

## Affected users and systems

CLI JSON output, `magelift ci` / build protocol, config schema generation, any Magento-adjacent JSON Magelift writes. Tests that match error strings.

## Constraints

Do not set `GOEXPERIMENT=nojsonv2` to hide breakage. Do not put secrets in JSON logs. PHP 8.2 package JSON is out of this Go change. Do not treat stdlib `uuid` as in-scope (OVH OKMS still uses `github.com/google/uuid`).

## Out of scope

`strings.CutLast` on GCP resource names and image digest suffixes (not equivalent to `LastIndex+1` when the separator is missing; use `filepath.Base` if we touch those). Migrating PHP JSON. Changing the YAML config language.

## Open questions

Which JSON documents are public contracts (must stay compatible) vs internal? Do we migrate every `encoding/json` import or only the CLI and protocol surfaces first?
