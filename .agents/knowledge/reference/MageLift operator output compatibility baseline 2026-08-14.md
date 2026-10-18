---
type: reference
title: MageLift operator output compatibility baseline 2026-08-14
description: Public command flags, output fields, and exit-code behavior that operator-facing changes must preserve or deliberately version.
tags: [cli, operations, compatibility, health, logs, cost, remote-access]
status: stable
generated:
  by: human:codex
  at: 2026-08-14
---

# MageLift operator output compatibility baseline

The root `--output` flag accepts `json`, `yaml`, and `table`. The table writer
currently serializes the same value as YAML, so table output is a stable
human-readable projection rather than a separate column renderer.

Exit codes are part of the operator contract: invalid input uses 2, provider
or runtime operation failures use 3, and a failed health check uses 4. A
successful command returns 0. Runtime health with unavailable evidence keeps
exit code 3. New degraded health reports use the failed-health code; stale
evidence uses the unavailable-evidence code.

`logs` accepts `--service web|deploy|cron`, `--since`, optional `--until`,
`--filter`, and a bounded `--limit`. A complete result identifies the selected
environment and log group and reports `status: complete`; a multi-source read
that loses one source reports the events it did read with `status: partial` and
returns exit code 3. Filters are provider-native for CloudWatch and literal
substring matches for Kubernetes-backed runtimes. Log messages are redacted at
the platform boundary and again before CLI output.

`cost` keeps account-free capacity classification as the default and requires
`--live` for provider price lookups. Reports now label the evidence class,
source, scope, observation time, and freshness. A configuration budget is not
represented as provider enforcement until an owned provider budget is read or
mutated through a verified adapter.

`exec` and `ssh` preserve argv boundaries and support `--session-only`. Session
previews must describe the launcher and target without printing kubeconfigs,
tokens, signed URLs, or secret values. Unsupported provider/runtime access
returns before a launcher is started.

When changing these contracts, add deterministic JSON, YAML, and table tests,
keep existing flags and successful exit codes stable, and document any
intentional output addition in the CLI reference.

# Related

See [MageLift generated CI previews must be PR-scoped](../lessons/MageLift%20generated%20CI%20previews%20must%20be%20PR-scoped%202026-08-14.md) for the analogous identity and lifecycle compatibility boundary.
