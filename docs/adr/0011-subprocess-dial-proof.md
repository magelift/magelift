# ADR 0011: Subprocess Dial proof for one adapter

- Status: Accepted
- Date: 2026-09-13

## Context

ADR 0008 left Magento deploy unwired through `Dial` and called that an
extract lag. The v1 plan needs one adapter proven over a subprocess while
the rest stay in-process: proof that trust, planning, and stack execution
work across the plugin boundary before wider extraction.

A Pulumi `RunFunc` is a Go closure. It cannot cross gRPC, so the existing
`Program` RPC can only report a kind. Stack work has to run inside the
provider process.

## Decision

- `gcp`/`gke-autopilot` is the v1 proof cell. The CLI uses a verified
  `magelift-provider-gcp` subprocess for it when one is installed beside
  the CLI executable (lockfile plus binary plus Cosign bundle).
- New `Execute` RPC carries `preview`, `up`, `destroy`, `outputs`,
  `redacted-outputs`, and `validate-request`. The subprocess runs Automation
  API with its own `gcpstack` program and returns change counts, outputs,
  and diagnostics. Results stay under 1 MiB; diagnostics truncate with a
  flag. The `outputs` op serves secrets decrypted (day-2 consumers need the
  real kubeconfig, matching in-process `Outputs`); only the
  `redacted-outputs` op redacts, for `magelift outputs` display.
- Preview ownership verdicts cross as data, not Go errors. The host
  rebuilds the typed conflict so the Runner classifies it like an
  in-process verdict.
- `Dial` pings the subprocess and refuses SDK API versions other than the
  host version before the session is returned.
- Every other provider and runtime stays in-process. A failed subprocess
  attempt falls back to the in-process backend with a stderr notice. It
  never fails the command.
- Single-version v1: the provider artifact version equals the CLI version.
  GoReleaser builds it on the same matrix and publishes the binary plus a
  Cosign bundle beside the CLI archives.
- Install layouts place the matching per-platform `magelift.providers.lock`
  beside the binary. CI generates the per-platform lockfiles from the
  release checksums at tag time; the generator and installer updates land
  with the v1 stable cut, not in this change.

## Consequences

- The host no longer imports provider SDKs on the proof path. `extensions
  list` reports the installed provider version, digest, and mode.
- Day-2 ports (`HasOps`, bootstrap, state, secrets, observe) stay
  in-process for every adapter, including the proof cell. Subprocess
  execution covers the stack lifecycle only; the port code runs in the host
  and reads its inputs (decrypted outputs, including kubeconfig) back over
  the `outputs` op on the local mTLS plugin channel.
- Release checksums cover the provider binaries. The first tag release
  verifies Windows asset naming and bundle upload; the smoke suite cannot
  cover those locally.
- Post-v1 extraction order: AWS ECS Fargate next (second certified
  origin), then the experimental providers, then community plugin
  onboarding.

## Alternatives considered

- Fat binary forever: rejected (size and community path, same as ADR 0008).
- `Program` RPC returning an executable handle: impossible across gRPC;
  execution has to live in the subprocess.
- Full extraction of every adapter in v1: rejected (release risk with no
  live proof yet; the proof cell de-risks it first).
- `binary_signs` for the provider bundle: rejected (it signs at build
  time, which breaks the offline local smoke; `signs` with
  `artifacts: binary` runs at release time only).

## Provenance

`internal/providerhost`, `cmd/magelift-provider-gcp`, `internal/cli`
subprocess wiring, `.goreleaser.yaml`. GoReleaser customization docs
(sign, binary archive format). Live Dial proof on GCP is Phase 2 and is
explicitly not claimed here.

## Scope note (ADR 0013, 2026-09-16)

Standing: provider-side Pulumi execution ("Stack work has to run inside the
provider process"), operations crossing as data, HashiCorp go-plugin
transport, and Dial-version pinging as the negotiation ancestor. Superseded
as target rules by ADR 0013: "Single-version v1: the provider artifact
version equals the CLI version", "A failed subprocess attempt falls back to
the in-process backend with a stderr notice. It never fails the command.",
and "Day-2 ports stay in-process for every adapter". Those lines stand as
the single-version current-state description; explicit negotiation,
fail-closed compatibility, and in-provider day-2 operations per 0013 are the
rule from acceptance forward, implemented by the follower intents (current
code still behaves the old way until Order 5 lands).
