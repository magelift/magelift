# Skills acceptance prompts

> All argv pinned against the CLI tree (`--help` + trial runs) 2026-09-16; no
> invented flags. The task-4 health URL is discovered from `local status`
> during the run (loopback port assigned at stack-up).

Setup for every task (fixture shop copy, installed skills only):

```sh
rm -rf /tmp/shop && mkdir -p /tmp/shop && cp tests/fixtures/skills-acceptance/magelift.yaml /tmp/shop/magelift.yaml
cd /tmp/shop
magelift skills install --agent generic --scope project   # [SEEN shape; exit 0 + clean:true proven in box 2.3]
```

The binary under test is the locally built `./cmd/magelift` (same embed as a
release build; version string `dev` is the only difference). The agent under
test reads ONLY `/tmp/shop/.agents/skills/*/SKILL.md` for Magelift procedure.

## Task 1 — configure (skill: magelift-configure)

```sh
magelift config validate --config magelift.yaml --env staging
magelift config effective --config magelift.yaml --env staging
```

Pass: both exit 0. `validate` prints `valid: true`. `effective` prints the
compatibility record (`status: supported`, `magentoVersion: 2.4.9`,
`phpBranch: "8.5"`), the resolved config, a `fingerprint: sha256:…` line, and
a `provenance:` map of per-key `source:` values (`project`, `environment
staging`, …).

## Task 2 — dependencies (skill: magelift-dependencies)

```sh
magelift doctor
```

Pass: exit 0 with `status: ok`. Output is a per-check list (`id`, `status`,
`message`, plus `capability`, `requirement`, `path`, `version` for tool
checks) separating required tools from optional capabilities; each tool
reports reachable (`available`) or explicitly missing. Must run from a dir
containing `magelift.yaml` (without one, doctor errors `open magelift.yaml:
no such file or directory`). Do NOT run `--install-dependencies` (skill:
read-only gate in acceptance).

## Task 3 — migrate (skill: magelift-migrate)

```sh
rm -rf /tmp/imp-acc /tmp/imp-ups
cp -r tests/fixtures/skills-acceptance/imports/acc /tmp/imp-acc
cp -r tests/fixtures/skills-acceptance/imports/upsun /tmp/imp-ups
cd /tmp/imp-acc && magelift init --from-acc --config-out ./acc.yaml
cd /tmp/imp-ups && magelift init --from-upsun --config-out ./ups.yaml
```

Pass: each `init` exits 2 with `import produced 1 unmapped
key(s); see <sidecar>` (exit 2 is the designed refused-success, not a
failure); `acc.yaml`/`ups.yaml` are mapped `schemaVersion: 1` docs;
`acc.unmapped.md` lists `` `hooks.build` `` from `.magento.app.yaml`;
`ups.unmapped.md` lists `` `crons.shell-backup` `` from `.platform.app.yaml`;
`grep -in 'secret|token|password'` over both YAML outputs is empty.

## Task 4 — local-runtime (skill: magelift-local-runtime)

```sh
magelift doctor
magelift local init
magelift local up --service app
magelift local status
curl -sf <app-health-URL-from-status>   # [TBD: exact URL/port — record from status output at run]
```

Pass: `local init` resolves the compatibility row and writes ignored
mode-0600 `.magelift/local.env` without touching the cloud target (skill
text); `local up --service app` starts the app service; `local status`
reports healthy [TBD: exact healthy rendering — capture verbatim at run];
the Magento health endpoint answers HTTP 200 with empty body semantics per
the image contract (`GET /health` short-circuit). No `local seed` (needs
Composer deps + admin password + full Magento install; out of scope — the
spec scenario needs healthy status + served health endpoint only). Requires
Docker; if absent, skip with reason per plan risks (does not fail the intent).

## Task 5 — operate (skill: magelift-operate, against the task-4 stack)

```sh
magelift local status
magelift local logs --service app
magelift local exec --service app -- bin/magento cache:flush
```

Pass: all three exit 0 [command shapes are skill-verbatim and drift-guard
verified against the Cobra tree; exit-0 confirmation captured at run].
RISK (record honestly at run): `bin/magento` needs Magento source inside the
app container, which the source-less fixture shop may not provide. If the exec
fails for missing Magento code (not for CLI/skill reasons), record a skip with
that reason instead of a pass; per plan risks, the three offline tasks
(configure, dependencies, migrate) are the must-pass floor.
