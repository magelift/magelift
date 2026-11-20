# ADR 0011. Run provider plans in a subprocess

Accepted on 2026-09-13. The initial implementation exercised GKE Autopilot.
[ADR 0013](0013-provider-plugin-contract.md) describes the current protocol.

A Pulumi program is a Go closure. The provider executable owns that program
and runs it via the Automation API. The CLI sends a plan or operation request
and receives change counts, outputs, and diagnostics. Ownership conflicts
cross the boundary as typed results.

The CLI verifies the provider executable, starts the process, and checks its
protocol version before use. Public output commands redact secret values.
A provider operation that needs a secret output uses it privately.
[Distribution trust](0014-distribution-trust.md)
