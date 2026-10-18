---
type: lesson
title: Fastly --token default is not the login session
description: Omitting `--token` uses the Fastly CLI login session; `--token default` selects a separately stored named token that can be expired after rotation.
tags: [fastly, credentials, cli, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
stale_after: 2027-02-13
---

# Rule

Do not pass `--token default` unless the operator explicitly selected the stored token named `default`. After `fastly auth login`, the working credential is the CLI's current session, selected by omitting `--token`. The named stored token `default` is a different object and can remain the old rotated token.

# Why

On 2026-08-13, `fastly service list --non-interactive --quiet --json` succeeded and listed one unmarked service. The same command with `--token default` returned HTTP 401. `FASTLY_API_TOKEN` was unset. MageLift's adapter and acceptance wrapper had defaulted `MAGELIFT_FASTLY_TOKEN_NAME` to `default`, so live Fastly cells would still fail after a working login.

Fastly CLI v15 documents `--token` as "API token, or name of a stored auth token (use `default` for the default token)". That wording is easy to misread as "always pass `--token default`". Empirically it selects the named stored token, not the login session.

# MageLift

`fastlyCLIArgs` and `scripts/fastly-acceptance-local.sh` pass `--token NAME` only when `MAGELIFT_FASTLY_TOKEN_NAME` is set. `FASTLY_API_TOKEN` still wins and omits `--token`. Never run `fastly profile list`, `fastly auth list`, `fastly auth show`, or `fastly auth token`; those can print the secret.
