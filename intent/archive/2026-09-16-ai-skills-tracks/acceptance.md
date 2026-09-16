# Skills acceptance run — 2026-09-16

Agent session following ONLY the installed user skills
(`/tmp/shop/.agents/skills/*/SKILL.md`, installed via `magelift skills
install --agent generic --scope project` from a local `./cmd/magelift` build —
same embed as a release build). Fixture shop: copy of
`tests/fixtures/skills-acceptance/magelift.yaml`. Prompts:
`tests/fixtures/skills-acceptance/prompts.md`.

## Task 1 — configure (magelift-configure): PASS

```sh
magelift config validate --config magelift.yaml --env staging
# environments: [- staging] / valid: true / exit 0
magelift config effective --config magelift.yaml --env staging
# compatibility: status supported, magentoVersion 2.4.9, phpBranch 8.5 / full
# resolved config / fingerprint: sha256:e5ec… / provenance: per-key source map
# exit 0
```

## Task 2 — dependencies (magelift-dependencies): PASS

```sh
magelift doctor
# config: magelift.yaml / status: ok / exit 0
# checks: build ok, environment.staging ok, local.edge ok,
# dependency.pulumi/docker/docker-compose/aws/session-manager-plugin ok —
# each with capability, requirement (required vs optional), path, version.
```

No missing tools on this host; `--install-dependencies` correctly not run
(skill: read-only gate).

## Task 3 — migrate (magelift-migrate): PASS

```sh
magelift init --from-acc --config-out ./acc.yaml
# import produced 1 unmapped key(s); see acc.unmapped.md / exit 2
magelift init --from-upsun --config-out ./ups.yaml
# import produced 1 unmapped key(s); see ups.unmapped.md / exit 2
```

`acc.unmapped.md` lists `` `hooks.build` `` (`.magento.app.yaml`);
`ups.unmapped.md` lists `` `crons.shell-backup` `` (`.platform.app.yaml`).
Both mapped YAMLs are `schemaVersion: 1`, no secret values (grep clean).
Exit 2 is the designed refused-success (sidecar present), per skill.

## Task 4 — local-runtime (magelift-local-runtime): PASS WITH WORKAROUND

```sh
magelift doctor            # exit 0 (as task 2)
magelift local init        # status: created / exit 0; writes ignored
                           # mode-0600 .magelift/local.env + compose.local.yml
magelift local up --service app   # see workaround note below
magelift local status      # all 6 services Up (healthy); app on 127.0.0.1:8080
curl -sf http://127.0.0.1:8080/health   # HTTP 200, body OK
```

WORKAROUND (test equipment, /tmp only, nothing committed): the generated
compose references `magelift/php-nginx:8.5-local`, which no documented local
flow produces (only `:local` exists; findings F1). Aliased
`docker tag magelift/php-nginx:local magelift/php-nginx:8.5-local` after
verifying `:local` is PHP 8.5.10 (byte-equivalent for this fixture); removed
the alias after the run. SECOND workaround, same run: the generated database
healthcheck runs `mysqladmin ping`, absent from MariaDB 11.8
(`mariadb-admin` exists); patched the /tmp compose copy to `mariadb-admin`
(findings F2). With both workarounds the documented skill procedure
(`init` → `up` → `status` → health endpoint) passes verbatim.

## Task 5 — operate (magelift-operate, task-4 stack): PARTIAL

```sh
magelift local status      # exit 0, all healthy (see task 4)
magelift local logs --service app
# exit 3: `no such service: mailpit` — PRODUCT BUG (findings F3). The flag
# value is ignored; compose resolves against a profile-less model.
magelift local logs        # exit 0 BUT output covers only cache-1/database-1
                           # (profile-less services); app/search/queue/mailpit
                           # logs silently omitted — same bug.
magelift local exec --service app -- bin/magento cache:flush
# exit 127: bin/magento: no such file or directory — HONEST SKIP (prompt risk
# note): the source-less fixture shop provides no Magento code to the app
# container, so no Magento command can run. Not a CLI or skill failure.
```

Task verdict: status PASS; logs PARTIAL (unfiltered works, filtered + gated
coverage broken by F3); exec SKIP (documented fixture limitation, predicted
in the prompt).

## Findings for the product backlog (out of this intent's scope)

- F1 — local stack references an untagged image: compose defaults
  `MAGELIFT_LOCAL_APP_IMAGE` to `magelift/php-nginx:8.5-local`, but no
  documented local flow builds branch-qualified tags (`make image-test`
  builds `:local`). Either build matrix tags locally or default the local
  reference to a produced tag.
- F2 — database healthcheck uses a removed binary: generated compose tests
  `mysqladmin ping`, but MariaDB 11.8 ships only `mariadb-admin`
  (`internal/localdev/compose.go:28`, `internal/localdev/catalog.go:166`).
  `local up` can never go healthy without editing generated output. One-word
  fix, own change + re-verify.
- F3 — `local logs`/`local down` ignore compose profiles:
  `internal/localdev/compose.go` `ComposeArgs` passes `--profile` flags only
  for `up`, so `logs --service app` errors (misnaming mailpit), unfiltered
  `logs` omits gated services, and `down` leaves gated containers running
  (verified: 4 survivors force-removed post-run). `status`/`ps` and `exec`
  resolve live containers and are unaffected.

## Tally

Tasks 1–3 FULL PASS (the must-pass offline floor). Task 4 PASS with two
documented /tmp-only workarounds. Task 5 PARTIAL (1 pass, 1 bug-blocked
partial, 1 honest skip). Scratch cleaned: stack down (plus 4 force-removed
stragglers, network removed), `/tmp/shop*` removed, tag alias removed,
installed skill copies removed with the fixture dirs.
