---
status: specified
slug: dial-proof-followups
intent: intent.md
---

# Spec: close the Dial proof test gaps

Auto-approved per the standing `/goal` instruction. Test-only intent;
no behavior change.

## Requirements

### Requirement: subprocess Plan equals in-process Plan

A `Plan` over a real Dialed GCP subprocess SHALL equal the
in-process `gcpstack.Module.Plan` for the same fixture input,
compared as JSON-decoded values (opaque spec included).

#### Scenario: Autopilot plan round trip

- **WHEN** the same GCP staging fixture is planned in-process and
  over Dial
- **THEN** both `ModulePlan` values decode to the same structure

### Requirement: Dial refuses version mismatch

`Dial` against a plugin binary reporting a non-`v1` SDK API version
SHALL fail with `ErrUnsupportedAPI` and kill the subprocess.

#### Scenario: Dial-level mismatch

- **WHEN** Dialing a helper plugin that pings `v0-test`
- **THEN** Dial returns `ErrUnsupportedAPI` and no session

## Design

- Plan test lives in `internal/providerhost`, reuses
  `buildGCPProvider` plus `writeVerifiedLock` plus `Load`, and
  imports `gcp/stack` plus `platform` test-only (no cycle; neither
  imports `providerhost`). The request `Configuration` is the JSON
  form of the fixture config, matching `platform.configMap`'s
  contract. Comparison unmarshals both plans to `any` so map key
  order cannot false-fail.
- Mismatch test uses the re-exec helper pattern: `TestMain` serves
  a wrong-version API when `MAGELIFT_TEST_FAKE_PLUGIN_VERSION` is
  set, and the test Dials `os.Executable` with that variable. The
  fake embeds the existing `fakeExecuteAPI` with a `Ping` override.

## Gotchas / policy flags

- No behavior change; if a test fails on a real divergence, fix the
  code and record it as a Deviation, do not weaken the test.
- The TestMain helper path must never run the suite; it serves and
  blocks until the parent kills it.

## Open questions carried forward

None.
