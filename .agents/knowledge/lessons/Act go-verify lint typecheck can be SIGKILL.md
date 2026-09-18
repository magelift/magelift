---
type: lesson
title: Act go-verify lint typecheck can be SIGKILL
description: Local `act -j go-verify` pulls lint (`./...`); golangci typecheck of pulumi-gcp can SIGKILL in Colima when disk/RAM are tight; leftover act containers keep gigabytes.
tags: [act, golangci-lint, pulumi, disk]
status: stable
env: [macos]
generated:
  by: cursor-grok
  at: '2026-08-19'
---

`go-verify` `needs: [changes, cache-prime, lint]`. `./scripts/ci-act-go.sh go-verify` therefore runs the unpartitioned lint job (`golangci-lint run ./...`). On Colima that typechecks `github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute` and can die with `compile: signal: killed` (OOM or a full data volume). That is not a product type error. Cap Go with `GOMEMLIMIT` at 75% of available RAM (`scripts/go-memlimit.sh`); do not pin `GOMAXPROCS` or `-p`.

`go-verify` also runs `gofmt -w` then `git diff --exit-code`, so a dirty worktree fails even when format is fine.

After a failed Act run, stop leftover `catthehacker/ubuntu:act-latest` containers. Do not stop unrelated containers. If the Mac data volume is ~100% full, `go clean -cache` is the large reclaim (tens of GiB). Substitute a host `golangci-lint run` plus Floci/harness — not another Act lint while disk is tight.
