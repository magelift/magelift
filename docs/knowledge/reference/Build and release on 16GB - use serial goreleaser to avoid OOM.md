---
type: knowledge
title: Build and release on 16GB - use serial goreleaser to avoid OOM
description: 'Even **serial** host-only builds can thrash under Cursor: a 2026-07-22 `--single-target
  --parallelism=1 GOMAXPROCS=1` smoke still pushed swap ~2.3 GB while compiling `pulumi-gcp/.../compute`
  (huge ...'
tags:
- build
- release
- goreleaser
- memory
- oom
- cursor
- tooling
generated:
  by: cursor/alexandres-macbook-air
  at: '2026-07-22'
created: '2026-07-22'
---

**On this 16 GB Mac, run magelift release/snapshot builds SERIALLY. The default parallel cross-compile exhausts RAM and kernel-panics / gets Cursor force-quit by macOS.**

## The trap
`scripts/release-smoke-local.sh` originally ran `goreleaser release --snapshot`, whose build matrix is `goos: [linux, darwin, windows] × goarch: [amd64, arm64]` = **6 targets**. GoReleaser builds targets **in parallel by default**, and each `go build` spawns up to **GOMAXPROCS (=10 here)** concurrent compilers. That's dozens of ~1 GB `compile`/`link` processes at once — far past 16 GB. Result on 2026-07-22: repeated JetsamEvents, two kernel panics, and macOS force-quitting Cursor for using all RAM+swap. Orphaned `go tool compile/link` processes (reparented to PID 1) then held ~4.35 GB even after the crash.

Even **serial** host-only builds can thrash under Cursor: a 2026-07-22 `--single-target --parallelism=1 GOMAXPROCS=1` smoke still pushed swap ~2.3 GB while compiling `pulumi-gcp/.../compute` (huge single package). Aborted to avoid another panic.

## The rule (agents: do this before any magelift build/release)
- Canonical local path: `./scripts/release-smoke-local.sh` / `make release-smoke` — `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `goreleaser build --snapshot --single-target --parallelism=1`. Never raise parallelism; never run the full 6-target matrix locally.
- Repo locks: `.cursor/rules/serial-builds-only.mdc` (alwaysApply), `AGENTS.md`, comments in `.goreleaser.yaml`.
- Prefer host-only smoke. Full matrices belong on GitHub Actions.
- Do NOT launch any magelift compile from inside Cursor while memory is already tight or while parallel subagents are active — run the binary smoke in a plain Terminal when idle.
- If the machine goes slow after a killed build, reap orphans: `pkill -9 -f 'libexec/pkg/tool/darwin_arm64'` (safe when no build is intentionally running).

## Why not just add swap
macOS swap is dynamic (grows to free disk; 53 GB free here, so disk was never the limit). More swap doesn't prevent the panic — it thrashes the SSD and the memory-pressure watchdog still fires. The fix is bounding peak memory, not swapping more.

## Related
Serena LSP is fine here (one shared daemon, ~2.9 GB gopls shared across subagents) — it was NOT the cause of these crashes. See cclsp to Serena daemon - per-subagent stdio LSP servers cause OOM for the separate LSP-multiplication issue that was fixed earlier.
