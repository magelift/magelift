---
type: lesson
title: Unsigned provider lock entries are refused before exec
description: magelift.providers.lock requires a sha256 digest plus Cosign identity and issuer. VerifyLocal hashes the binary and checks the Cosign blob before any go-plugin spawn. plugin.Open is forbidden.
tags:
- providerhost
- cosign
- go-plugin
- openspec
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: '2026-08-18'
---

`internal/providerhost` parses `magelift.providers.lock` (`schemaVersion` 1, `sdkAPIVersion` `v1`). Each provider entry needs `name`, `version`, `digest` (`sha256:` + 64 hex), and Cosign `identity` plus `issuer`. Missing signature is `ErrUnsigned`. Missing or malformed digest is `ErrDigestRequired`. `VerifyLocal` hashes the file (1 GiB cap) against the lock digest, then calls the injected Cosign blob verifier. It does not execute the binary. `Load` with `ModeInProcess` skips the lock (tests and Floci). `ModeSubprocess` verifies first; `Dial` then starts HashiCorp go-plugin over gRPC (`cmd/magelift-provider-gcp` Ping, GCP Autopilot Describe, and `sdk.Module` Plan/Program JSON). Program returns a `pulumi.RunFunc` kind and does not execute it. Magento cells stay in-process until the CLI is wired to `Dial`. Go `plugin.Open` is not used ([ADR 0008](../../../docs/adr/0008-provider-load-path.md)). A fat Pulumi-linked plugin exceeded a 256 MiB hash cap and mismatched the lock digest; VerifyLocal now hashes up to 1 GiB.
