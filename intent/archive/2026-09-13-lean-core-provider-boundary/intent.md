---
status: done
slug: lean-core-provider-boundary
---

# Intent: lean core with a provider boundary v1 can freeze

## Problem

The vision is a lean CLI core holding generic logic, providers behind an interface layer as plugins, each with its own release lifecycle, plus community plugins for more Pulumi providers. Today first-party providers link in-process into one binary, the signed-subprocess host (`internal/providerhost`) proves only Ping/Describe, and Magento deploy stays in-process until Dial is proven. The maintainer decided v1 proves Dial for one adapter instead of blocking on full extraction: the plugin path becomes real without dragging a full version matrix into v1.

## Evidence

`docs/adding-a-provider.md`: released CLI "will load" signed subprocess artifacts; "Magento cells stay linked in-process until the CLI is wired to Dial (one-release ceiling)." The host already serves Ping, GCP Autopilot Describe, and `sdk.Module` Plan/Program JSON RPCs, which makes GCP Autopilot the natural Dial proof candidate. `openspec/specs/provider-extension-loading/spec.md` requires dual-mode loading and independent provider releases, but marks "Lean core and community catalog" as P2. `sdk/v1` plus `platform.StackModule` plus `cli.NewWithExtensions` already give a compile-time community path proven by `examples/custom-extension-contract` with empty caches (`make extension-test`). `internal/cloud/<provider>/` layout follows ADR 0003/0004. Whether any third party has built a community module from the docs alone: not checked.

## Proposed outcome

V1 freezes the SDK boundary and proves signed subprocess Dial end to end for one first-party adapter: provider code runs in a verified subprocess for plan plus deploy plus destroy on that adapter, with Cosign identity plus lockfile digest plus SDK API version enforced before any provider code runs. The remaining adapters stay in-process for v1. Per-provider independent releases still land post-v1 without breaking the frozen YAML/CLI contract. The decision and its sequencing are recorded in an ADR so contributors stop re-litigating it.

## Affected users and systems

Provider authors (first-party and community), `sdk/v1`, `internal/platform`, `internal/registry`, `internal/providerhost`, `examples/custom-cli`, `examples/custom-extension-contract`, `docs/adding-a-provider.md`, `docs/versioning.md` RC stability statement.

## Constraints

No Go `plugin.Open`, no unsigned remote loader, no working-directory auto-discovery. Pulumi SDKs stay out of `internal/cli` so gendocs and CLI unit tests link on small runners. Tests, Floci suites, and acceptance harnesses stay in-process; subprocess runs only on the published-CLI path for the proof adapter. Shared-kube Observe/Steps consolidation may move `platform` port wiring during RC per the versioning reservation; callers must not treat those ports as frozen. Smallest change that freezes the boundary and proves Dial once; no speculative plugin framework.

## Out of scope

`magelift.providers.lock` lifecycle, community catalog hosting, new provider SDKs, independent per-provider releases.

## Open questions

Which adapter is the Dial proof candidate (GCP Autopilot recommended since the host already serves its RPCs), and what must go through Dial for the proof to count: plan only, or plan plus deploy plus destroy? What compatibility test makes the SDK freeze enforceable rather than documentary?
