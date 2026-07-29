# 03-06 Paid AWS acceptance proof

**Status:** template — fill after HUMAN_GATE spend approval. Do **not** invent PASS rows by hand.

Evidence files stay under gitignored `.magelift/` (checkpoint + `matrix-results.md`). This scratch note and docs cite harness output only.

## Required env

| Variable | Purpose |
|----------|---------|
| `MAGELIFT_BIN` | Built magelift binary |
| `MAGELIFT_CONFIG` | Acceptance `magelift.yaml` (preview free-tier; include `queueSecretArn` for ecs broker cells) |
| `MAGELIFT_AWS_ACCEPTANCE_DIGEST` | Signed immutable runtime image digest |
| `MAGELIFT_AWS_CERTIFICATE_IDENTITY` | Expected Sigstore certificate identity |
| `AWS_REGION` / `AWS_DEFAULT_REGION` | Region for apply + assert_clean |
| `MAGELIFT_AWS_ACCEPTANCE_PROFILE` | Default `preview` (do not set `ALLOW_COSTLY` for this pass) |

Optional: `MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER` (defaults to GitHub Actions OIDC issuer).

## Cell catalog (≥3, free-tier only)

From `scripts/acceptance/cells-aws-preview.txt`:

1. `queueMode:db`
2. `queueMode:ecs-rabbitmq`
3. `queueMode:ecs-artemis`

**Excluded:** Aurora apply, `amazon-mq`, OpenSearch apply.

## Procedure (KEEP=true create-once)

Offline preflight first (no spend):

```sh
MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash tests/acceptance/checkpoint_resume_test.sh
bash tests/acceptance/evidence_append_test.sh
MAGELIFT_ACCEPTANCE_AWS_STUB=1 bash tests/acceptance/assert_clean_stub_test.sh --clean
bash tests/acceptance/matrix_tier_guard_test.sh
# or: make acceptance-harness-test
```

Live long-lived stack (after spend approval):

```sh
export MAGELIFT_BIN=/path/to/magelift
export MAGELIFT_CONFIG=/path/to/acceptance.magelift.yaml
export MAGELIFT_AWS_ACCEPTANCE_DIGEST='ghcr.io/…@sha256:…'
export MAGELIFT_AWS_CERTIFICATE_IDENTITY='…'
export AWS_REGION=eu-north-1   # or your free-tier region
export MAGELIFT_AWS_ACCEPTANCE_KEEP=true

./scripts/aws-acceptance-local.sh
```

**What the live path does**

1. Logs `acceptance create-once` once → `preview` + `promote` + initial `deploy` / `outputs` / `health`.
2. For each incomplete catalog cell: logs `acceptance cell-update cell=…`, patches `queueMode` into a **temp copy** of `MAGELIFT_CONFIG` (via `yq`; does not mutate the source file), then `deploy --digest … --yes` + `outputs` + `health`.
3. Appends `.magelift/matrix-results.md` and records checkpoint PASS/FAIL per cell.
4. With `KEEP=true`, EXIT trap skips destroy (stack stays up for kill+resume / leftover assert).

## Kill + resume

1. After ≥1 cell `PASS` in checkpoint, interrupt the script (Ctrl-C) mid-matrix.
2. Confirm DIY lock hygiene if deploy was mid-flight (unlock only when account empty — see `docs/aws-acceptance.md`).
3. Re-invoke with the same env plus KEEP (and optionally `MAGELIFT_AWS_ACCEPTANCE_RESUME=1`):

```sh
export MAGELIFT_AWS_ACCEPTANCE_KEEP=true
export MAGELIFT_AWS_ACCEPTANCE_RESUME=1   # optional if checkpoint already has cells
./scripts/aws-acceptance-local.sh
```

Expect: `acceptance resume: skipping create-once`, `acceptance skip cell=…` for PASS cells, then `cell-update` for the first incomplete cell.

## Evidence sample (paste redacted harness row)

After a live run, copy one row from `.magelift/matrix-results.md` (account id OK; **no** digests/secrets):

```
| cell | result | duration | provider | account | date |
|------|--------|----------|----------|---------|------|
| queueMode:db | PASS | … | aws | <account> | YYYY-MM-DD |
```

## assert_clean dual outcome

1. **Leftover → non-zero:** With KEEP still true (or a deliberate tagged leftover), run `assert_clean` / finish a KEEP session without destroy — expect non-zero. Record command + exit code here.
2. **Clean → 0:** Unset KEEP (or destroy explicitly), re-run destroy path:

```sh
unset MAGELIFT_AWS_ACCEPTANCE_KEEP
# or: magelift --config "$MAGELIFT_CONFIG" --env preview destroy --yes
./scripts/aws-acceptance-local.sh   # only if resuming destroy; prefer explicit destroy + assert
```

Prefer safest leftover (tagged acceptance resource you can destroy immediately). Always finish with destroy + clean account.

## Proof checklist (fill after live)

| Criterion | Log / artifact proof | Done |
|-----------|----------------------|------|
| ACCEPT-01 create-once then ≥3 cell-updates, no re-create | | |
| ACCEPT-02 kill+resume skips PASS cells | | |
| ACCEPT-03 harness-written matrix-results six columns | | |
| ACCEPT-04 assert_clean leftover≠0 and clean=0 | | |

## HUMAN_GATE

- [ ] Spend approval recorded in chat: `approved — AWS free-tier spend OK for Phase 3 harness proof.`
- [ ] Live create executed only after that signal
- [ ] Account left clean after final destroy
