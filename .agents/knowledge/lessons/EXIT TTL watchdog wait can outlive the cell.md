---
type: lesson
title: EXIT TTL watchdog wait can outlive the cell
description: Killing only the watchdog subshell can reparent `sleep` to PID 1 and block EXIT `wait` for the rest of the TTL after the cell already passed.
tags: [acceptance, cleanup, shell]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
---

`acceptance_start_ttl_watchdog` backgrounds a subshell that sleeps then signals
the wrapper. `acceptance_stop_ttl_watchdog` used to `kill` that subshell and
`wait` it. On macOS the inner `sleep` can be reparented to PID 1, so EXIT
blocks until the original TTL elapses even after PASS.

Kill the subshell's children first, then the subshell, then wait. Do not treat
a wrapper process that is still alive after PASS as proof the cell is still
mutating the provider.

The 2026-08-14 GCP Cloud SQL cell exposed a second boundary: a 30-minute
operator TTL expired while source creation and isolated restore were still
running. The recovery command later printed its internal PASS result, but the
wrapper correctly returned failure because the TTL had already fired. Exact
cleanup removed both instances and the backup. Set the TTL from the slowest
provider operation budget plus cleanup time; the shared default is six hours.
