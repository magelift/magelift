# ADR 0008: In-process Magento, signed go-plugin for published adapters

- Status: Accepted
- Date: 2026-08-22

## Context

The default binary must not download unsigned plugins. Go `plugin.Open` is ABI-fragile and weak on Windows. Magento deploy still needs the AWS and GCP adapters in the same process as the CLI for RC1.

## Decision

- Magento cells load first-party adapters **in-process**. Tests do the same.
- The published CLI path for extracted adapters is `magelift.providers.lock`: sha256 digest plus Cosign identity and issuer. `VerifyLocal` hashes the binary and checks the Cosign blob before any HashiCorp go-plugin spawn. Unsigned entries are refused.
- Go `plugin.Open` is forbidden.
- Community providers are compile-time custom binaries (`examples/custom-cli`). They are not auto-downloaded.
- In-process `Load` skips the lock (tests and Floci). `ModeSubprocess` verifies first, then `Dial` starts gRPC. The `gcp`/`gke-autopilot` proof cell deploys through `Dial` when a verified artifact is installed ([ADR 0011](0011-subprocess-dial-proof.md)); every other Magento cell stays in-process until its extraction lands.

## Consequences

Homebrew can ship core while a GCP-only project downloads `magelift-provider-gcp` once that host exists. Adapters may stay in-process for one RC1 release if extract lags. `plugin.Open` stays out.

## Alternatives considered

- Fat binary forever: rejected (size and community path).
- `plugin.Open`: rejected (ABI, Windows).
- Unsigned remote install: rejected.

## Provenance

`internal/providerhost`. HashiCorp go-plugin and Cosign public docs.
