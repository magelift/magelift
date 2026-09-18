# rc12 corrections follow-up

Reviewed 2026-09-18, `09bc482..53452ae` (six commits). Verdict: **the runtime security fix passes; promotion still has two fail-open cases.** Continue the deployment exercise, but do not treat these changes as approval to promote or close acceptance.

## Remaining findings

### 1. P1 — A pending newer validation run falls back to an older success

Location: `scripts/release-checks-gate.sh:192–218`.

The gate sorts candidate runs newest first. When the newest run has pending checks, it prints PENDING, sets `chosen_pending`, and continues to older runs. An older complete success then exits zero before the pending state is considered. A newer run without an aggregate check yet is excluded from the candidate list altogether.

Reproduction: run 100 has successful aggregate, provider, and both PHP checks; run 200 has queued aggregate and provider checks. The gate prints:

```text
PENDING  CI passed: status=queued (attempt 200, run 200)
PASS     CI passed: success (run 100)
PASS     provider-verify: success (run 100)
PASS     php (8.3): success (run 100)
PASS     php (8.5): success (run 100)
exit: 0
```

This can publish while the newer validation is still running and may subsequently fail. Existing tests cover a pending attempt within one run, but not a pending newer workflow run alongside an older green one.

Select the intended validation run once, preferably using an explicit run ID and verified commit/workflow identity. Evaluate only that run. Pending must remain pending; missing checks must not trigger a search for an older success. Add the two-run regression case, including the interval before the newer aggregate check exists.

### 2. P1 — A successful aggregate can conceal skipped release-required checks

Location: `scripts/release-checks-gate.sh:26`, together with `.github/workflows/ci.yml`'s aggregate policy.

The new default list requires only `CI passed`, `provider-verify`, and PHP 8.3/8.5. The aggregate deliberately accepts path-filtered skipped jobs. Consequently the new gate no longer requires the actual root Go, SDK, lint, and GCP contract execution that the previous gate demanded.

I supplied one run with those four new requirements successful and `go-verify`, `sdk-verify`, `lint`, and `floci-gcp` explicitly skipped. The gate printed four PASS lines and exited zero. This is consistent with the aggregate's real policy, not an impossible aggregate state. For example, provider-manifest and PHP-only changes can activate provider/PHP validation without activating all the other families.

Retain the aggregate to catch failures elsewhere, but also require success for every mandatory release-validation job on the selected run. Alternatively, introduce a release-validation aggregate that requires the full job set and rejects skips. A comment telling operators to dispatch `all=true` does not enforce that condition. Add a fixture where the aggregate succeeds but a mandatory underlying job is skipped.

## Disposition of the preceding review

| Finding | Result |
| --- | --- |
| nginx private-media PHP execution | Fixed in the reviewed configuration. The HTTP harness passed sensitive-directory denies, ordinary-media script denial, theme XML denial, and positive `get.php` routing. |
| Promotion ignored failed aggregate / missing PHP cells | Those specific fixtures now pass, and pagination and same-run grouping were added. The two cases above keep the broader finding open. |
| Nested provider archive omitted LICENSE | Fixed for this repository. All five modproxy tests passed, including comparison of all three staged rc11 module checksums with published modules. |
| Acceptance runbook weakened project-lock and generated-CI proofs | The intended proofs are restored in writing. Execution evidence is still required. |
| Release cache was lookup-only | Corrected to restore before compilation and save-only afterward. Workflow execution, rather than this source review, will establish actual cache hits. |

Adding `provider-verify` to `CI passed`'s dependencies also closes the aggregate omission noted previously. The nginx fix changes the actual selection rules rather than only adding more denied path strings.

Two verification details should be tightened without expanding product scope:

- The published-module equivalence test skips if the rc11 tag is absent. The Go CI checkout uses the default shallow checkout, so this test is not guaranteed to execute there. Fetch the specific reference in a dedicated release-preflight job or use a deterministic equivalence fixture; keep ordinary unit tests independent of public network availability.
- Runbook step 10 creates a conflicting beside-CLI lock but does not explicitly place a conflicting provider executable beside the CLI. To prove rejection of the wrong beside-CLI executable, stage that executable too and record the actual selected process/version. Installation alone must not substitute for the stated execution evidence.

The new HTTP harness tests the checked-in nginx configuration with official nginx/PHP containers. It is useful behavioral coverage, but it does not run each freshly built MageLift image from the surrounding PHP matrix. Its comment mentions an optional image config-source check that is not implemented. Preserve the clean-machine test against the actual candidate image so source configuration and packaged-image behavior are both covered.

## Evidence and limits

Commands executed successfully:

```sh
GOMAXPROCS=8 GOFLAGS=-p=4 GOMEMLIMIT=4GiB \
  TMPDIR=/home/dev/.cache/magelift-alpha-review-tmp \
  go test ./cmd/modproxy -count=1 -v

TMPDIR=/home/dev/.cache/magelift-alpha-review-tmp \
  bash scripts/nginx-media-deny-test.sh

bash tests/acceptance/release_checks_gate_test.sh
bash tests/acceptance/nginx_media_deny_shape_test.sh
```

Results: five Go tests passed; the nginx HTTP harness passed; both shell fixture suites passed. Two additional generated payloads reproduced the promotion defects above. Test fixtures were written under the review temporary directory; product source was not changed. HEAD remained `53452ae` during verification.

At the final workflow inspection, [rc12 release run 35335610055](https://github.com/magelift/magelift/actions/runs/35335610055) was still in progress at “Wait for module visibility”, on `53452ae`. I did not observe a completed rc12 release or completed clean-machine acceptance evidence. This review did not access or mutate the acceptance cloud resources, stop another agent's work, dispatch CI, or publish anything.

The next change should stay confined to selecting and enforcing complete release validation. Rerun its failure fixtures, then preserve the actual candidate's deployment and generated-CI evidence before closing boxes 1.4 and 5.1.

## Follow-up verification: `31f6721`

Both P1 findings above are resolved in `98a7a67`, reviewed through `31f6721`. The release workflow selects a CI workflow run for the target commit and passes its ID to the gate. The gate evaluates that run without falling back, and explicitly requires the mandatory checks to succeed.

The full release-gate fixture suite passed. Independently rerunning the two original reproduction payloads returned the expected results: the newer pending run exits 2; skipped lint, root Go, SDK, and GCP contract checks exit 1. `git diff --check` passed. The runbook now also requires a conflicting beside-CLI executable and actual provider execution evidence.

This closes the two source-review findings, not the acceptance proof. At verification time the rc12 tag and running release workflow still pointed to `53452ae`, which predates these fixes. The running workflow therefore does not contain the corrected promotion gate. Apply the reviewed gate as a separate explicit verification of the candidate's CI before any publication decision; future release tags should include the workflow fix. Do not move the existing tag to imply it contains later changes.
