# MageLift agent notes

## Hardware constraint (this Mac)

**Never build in parallel locally.** Parallel goreleaser / `go build` / multi-platform matrices exhaust RAM+SWAP and have caused kernel panics on this 16 GB machine.

- Use `./scripts/release-smoke-local.sh` or `make release-smoke` only (`--single-target --parallelism=1`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`).
- Full release matrices run on CI (`ubuntu-latest`), not here.
- If swap climbs or the machine crawls during a build: abort, reap orphans (`pkill -9 -f 'libexec/pkg/tool/darwin_arm64'` when no intentional build is running), retry later in a plain Terminal.

See `.cursor/rules/serial-builds-only.mdc` and the vault note *Build and release on 16GB*.
