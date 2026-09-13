---
status: draft
slug: cutlast-cli-image-refs
---

# Intent: strings.CutLast at CLI image and resource refs

## Problem

Go 1.27 added `strings.CutLast`. Magelift's CLI still peels image tags, digests, and last path segments with `LastIndex` / `LastIndexByte` arithmetic. That is easy to get wrong at the registry-port vs tag boundary (`host:5000/repo:tag`) and it is the same class of debt as wrapping Pulumi errors: operators feel it when `magelift build --image` or printed digests are malformed. We skipped it in the 1.27 bump because CutLast is not a one-line substitute for `s[LastIndex(sep)+1:]` when the separator is missing.

## Evidence

`internal/cli/build.go` `digestReference` uses `strings.Cut` for `@`, then `LastIndexByte` for `/` and `:` so a tag is stripped only when the colon sits after the last slash. Collector and kube code uses `LastIndex(image, "@sha256:")` for digest pins the CLI and deploy path print. GCP secret/ledger helpers use `LastIndex(name, "/")+1`. Focused CLI tests passed after the bump; a CutLast rewrite of `digestReference` against `localhost:5000/...` fixtures: not checked.

## Proposed outcome

CLI-facing image references (build `--image`, digest pins, `@sha256:` pins the CLI prints) are split with `Cut` / `CutLast`, with tests for: tag vs registry port, missing separator (keep the whole string), and digest suffix. Wrong splits fail tests, not production. Helpers that only exist for `magelift` output or `--image` parsing live in CLI or a shared ref helper, not copied per provider.

## Affected users and systems

Operators of `magelift build --image` and digest-pinned deploys. `internal/cli/build.go`. Image digest parsing in kube/AWS/GCP collectors if the CLI surfaces those refs. Not Magento PHP.

## Constraints

`localhost:5000/repo:tag` must still treat `:tag` as the tag and `5000` as the port. Missing separator must not become an empty string. Cosign `ValidateReference` stays the gate on the assembled digest ref. Go 1.27 toolchain already required.

## Out of scope

`encoding/json/v2`. Changing which flags the CLI accepts. Rewriting URL parsing that already uses `url.PathUnescape`. stdlib `uuid`.

## Open questions

Do we stop at `digestReference` plus tests, or also move the `@sha256:` LastIndex sites the CLI prints? One shared `imageref` helper vs local CutLast at each call site?
