# Review of the rc12 preparation

Date: 2026-09-18. Verdict: **continue corrections; do not close this intent or approve the alpha yet.**

The changes address the substance of the previous review. Private GCS storage, the empty Magento remote-storage root prefix, rooted import reads, and the onboarding commit step are sensible corrections. However, the HTTP security boundary still fails an execution test, the promotion gate can approve a commit whose required CI failed, and the new module staging tool does not reproduce the published provider archive. The clean-machine runbook also weakens two agreed proofs.

This review covers the cumulative diff from the previous review baseline `6b67359` through `09bc482`: 47 files, including release workflows, module staging, module manifests, media provisioning and transfer, nginx, build guards, shell harnesses, onboarding, and intent changes. Application-code tests ran against an isolated archive of `1a98ac1`; subsequent commits through `09bc482` changed release manifests. The archive received a local synthetic Git commit solely because the modproxy tests require `git archive HEAD`. This avoids testing a worktree being changed by the release agent. No product code, release tags, published artifacts, or cloud resources were changed by this review.

## Findings

### 1. P1 — The new private-media denies are bypassed by the generic PHP handler

Location: `images/php-nginx/nginx.conf:44`, interacting with the handler at line 64. The new Makefile checks only verify that location strings exist.

The four directory denies are ordinary prefix locations. Nginx can select the subsequent regular-expression PHP location instead. An existing PHP file below a supposedly denied directory is sent to PHP-FPM and executed.

I ran the checked-in nginx configuration in the existing local runtime image, with no external network and no published ports. Two harmless files were created in the disposable container:

```text
GET /media/customer/probe.txt -> HTTP/1.1 403 Forbidden
GET /media/customer/probe.php -> HTTP/1.1 200 OK, body EXECUTED
```

The PHP file contained only `<?php echo "EXECUTED";`. The test used an explicit nginx worker user because the image has no default `nginx` user when launched as root. An initial invocation without that override failed to start nginx; it was not counted as evidence.

This does not establish an unauthenticated upload vulnerability. It establishes that a PHP file introduced through migration, a compromised extension, or another write path can execute from a directory the new configuration claims to deny. The broad PHP handler predates this patch, but the added protections are incomplete in its presence. The same handler also makes adding more prefix denies an insufficient general fix.

Restrict PHP execution to the application's approved entry points, deny other script files, and ensure private-media denies take precedence. Test the actual HTTP behavior for each sensitive subtree and for scripts in ordinary media directories. Keep a positive test showing that a missing public image still reaches `get.php`. Also review the omitted stock restriction on `media/theme_customization/*.xml` before claiming stock-equivalent protection.

Sources: [nginx location selection](https://nginx.org/en/docs/http/ngx_http_core_module.html#location) and [Magento 2.4.9 nginx sample](https://github.com/magento/magento2/blob/2.4.9/nginx.conf.sample). The Magento sample uses explicit PHP entry points and a separate script deny rule; copying only its directory blocks loses that protection.

### 2. P1 — Promotion does not require the complete CI result

Location: `scripts/release-checks-gate.sh:26`, `:123`, and its single-page check-runs request; `.github/workflows/release.yml`, “Publish the verified candidate”.

The matrix-name correction fixes the earlier inability to recognize `php (8.3)`. Selecting the highest check-run ID also fixes the queued-rerun ordering bug. But the gate still has two fail-open cases:

1. It accepts its six selected check families even if other required CI fails. I fetched the actual rc11 commit's checks (`f99f3a8`) and passed the payload to the current gate. It exited zero, printing six PASS lines, despite `CI passed`, `acceptance contracts`, and `shellcheck` having failure conclusions.
2. It accepts whatever PHP cells happen to be present. A fixture containing the five non-PHP successes and only `php (8.3)` also exited zero. Full dispatched CI defines PHP 8.3 and 8.5; push CI defines four versions. The absence of the other cells is not checked. A truncated API result has the same risk because `filter=all` returns historical attempts and the script reads only 100 entries.

The rc11 release run [35300776963](https://github.com/magelift/magelift/actions/runs/35300776963) failed promotion because its older gate did not recognize the PHP matrix names. GitHub nevertheless reports [rc11](https://github.com/magelift/magelift/releases/tag/v0.1.0-alpha.1-rc.11) as published at 04:30:53 UTC. That publication is not evidence of a green release workflow. I have not established who published it or which separate checks they performed.

Require a successful authoritative full-CI result for the intended release-validation run, while retaining the explicit provider check and checks that must not be skipped. Tie the result to that run and commit so successes from unrelated partial runs cannot be combined into a fictitious complete run. Handle pagination and pending reruns. Add fixtures for failed aggregate CI, missing matrix cells, and checks spread across pages. The current aggregate does not include `provider-verify`, so simply replacing the entire list with `CI passed` would introduce another gap.

### 3. P2 — The staged GCP module differs from the published module

Location: `cmd/modproxy/main.go:78` and `:132`; tests in `cmd/modproxy/main_test.go`.

The staging tool archives the GCP subdirectory and passes it to `modzip.CreateFromDir`. It does not inherit the repository-root `LICENSE`. Go's repository-to-module conversion does inherit that file when a nested module has none of its own.

I staged the already published rc11 commit with the new tool and compared its GCP ZIP against the existing cached published ZIP:

```text
Only published: github.com/magelift/magelift/providers/gcp@v0.1.0-alpha.1-rc.11/LICENSE
Only staged: []
Changed common files: []
```

The provider's content checksum therefore differs. The current tests prove internal consistency between the tool's own files, not equivalence with Go's published representation. SDK staging avoids this particular problem because `sdk/LICENSE` already exists.

Mirror Go's license inheritance when staging nested modules and add an equivalence regression test. Compare all three staged module content hashes against an already published version using isolated caches. This is a prepublication-tool defect; it does not by itself prove that the current rc12 root/SDK dependency sums are wrong, since those modules do not depend on the provider ZIP.

Source: [Go module repository conversion](https://go.dev/ref/mod#vcs-dir). Archive byte identity is also a stronger claim than needed: the important contract is identical module contents and Go checksums.

### 4. P2 — The clean-machine runbook substitutes weaker tests for the approved proof

Location: `intent/alpha-review-corrections/1.4-runbook.md:66`; compare `spec.md:20` and `spec.md:36`.

Step 10 calls the project-pinned repeat a `--project`-flag test. R01 requires proving that a project provider lock controls the version executed, including resolution against a beside-CLI binary and the versioned cache. Selecting a project is not that test.

The same step reduces the generated-CI repeat to lint/validate and checking for a sweep schedule entry. The spec requires the connected installation path to run through generated CI in a scratch organization repository. Linting cannot prove provider download, signature verification, production registry dispatch, or cloud identity in that runner.

Restore concrete steps and pass/fail evidence for both proofs. Record the selected lock, actual executed provider version, and generated workflow run URL. Keep box 1.4 open until those executions succeed. The newly detailed media checks are useful and should stay, with finding 1's HTTP negatives added.

### 5. P2 — Release builds never restore the cache they claim to warm

Location: `.github/workflows/release.yml:38–47` and `:236–242`.

The initial `actions/cache` call sets `lookup-only: true`. That checks cache availability without downloading it. `setup-go` also has caching disabled. Consequently the shared CI cache is not restored before compilation. The later ordinary `actions/cache` call runs after the expensive work; it is not an explicit save-only action and can restore an existing cache at that point.

Use the pinned cache restore action before compilation and a save-only action afterward with the intended failure policy. Verify restoration and saving in the workflow logs. Increasing GoReleaser timeouts and serializing its matrix can help memory pressure, but cannot recover the reuse lost here.

Source: [actions/cache inputs and separate restore/save actions](https://github.com/actions/cache#usage).

## Previous findings and overall direction

| Previous issue | Current assessment |
| --- | --- |
| Public bucket could expose Magento import/export files | Anonymous binding removed; public-access prevention enforced. Correct direction. Live upload and private-path proof remain open. |
| Magento and CLI disagreed on the media prefix | Empty remote-storage root and explicit CLI `media/` subtree now agree. Tests pass. |
| Import parent swap could read outside the source tree | Reads now use `os.Root`; containment and parent-swap tests pass. |
| Queued rerun could be hidden behind an older success | Highest check-run ID fixes the reported ordering defect. Broader promotion gaps remain in finding 2. |
| Onboarding generated an uncommitted provider lock before a clean-tree build | The documented commit step fixes that omission. |

Allowing private object ACLs while enforcing bucket public-access prevention addresses the earlier GCS compatibility concern in the design. It still needs the real Magento upload proof: Pulumi mocks cannot validate GCS XML API behavior or Magento's HMAC operations. Likewise, the new application-relative URL bindings and `get.php` route need retrieval from a fresh pod, not merely a warm filesystem or an object-existence check.

The Cosign size-cap increase preserves a bounded download and the existing trust policy. Staging from committed content and isolating staging caches are useful improvements. The changed shell harnesses passed their offline tests. Suppressing the costly prerelease container matrix at the trigger is reasonable for the stated candidate workflow, provided the documented candidate-image build remains part of acceptance.

I would retain the narrow GCP-first roadmap and the explicit deferral of provider import decoupling until the second autonomous provider. This diff does not justify another architectural rewrite. The immediate problem is release preparation discovering deterministic defects after tagging. Move archive equivalence and complete promotion checks into an executable preflight before spending another release cycle. The growing sequence of manual checksum-editing instructions is too easy to misapply; use the now-working staging approach as the basis for one tested preflight once its conversion is correct.

The roadmap still says order 8 reruns from rc2, and checked plan item 2.5 still describes rc2-era ordering that conflicts with the new pre-tag staging procedure. Update those during box 5.1 and link exact evidence instead of treating repeated “verified pristine” commit subjects as the verification report. Keeping 1.4 and 5.1 unchecked is honest; do not archive the intent before they pass.

## Verification performed

Local Go execution used the larger devbox limits authorized by the maintainer:

```sh
GOMAXPROCS=8 GOFLAGS=-p=4 GOMEMLIMIT=4GiB \
  go test ./cmd/modproxy ./internal/providerhost ./internal/build/pipeline ./internal/platform -count=1

# From providers/gcp:
GOMAXPROCS=8 GOFLAGS=-p=4 GOMEMLIMIT=4GiB \
  go test ./plugin ./storage ./runtime ./stack -count=1
```

The runner reported 217 passing tests in the first group and 106 in the second. Host PHP is absent, so I separately reran both `TestApplicationDockerfileGuard*` tests through a temporary PHP wrapper using the existing local runtime container. Both passed, including the PHP predicates that would otherwise be skipped.

All eight changed shell test scripts passed: lifecycle guards, AWS collector and database-recovery harness shapes, GCP Cloud SQL/collector/native-observability shapes, recovery guards, and release-check gate fixtures. The additional incomplete-matrix fixture and actual rc11 payload both demonstrated false gate success. The disposable nginx execution test demonstrated the private-directory bypass. Staged-versus-published rc11 archive comparison demonstrated the missing license.

This was a focused change review, not a full `make verify`, release-matrix run, clean-machine GCE deployment, or order-8 store acceptance. No rc12 proxy requests were made during preparation. Published rc11 metadata and checks were read through GitHub; module comparison used its already cached ZIP. Passing local tests does not certify rc12 artifacts or the live Magento path.

## Recommended next steps

1. Fix nginx execution restrictions and add HTTP tests before deploying this candidate for acceptance.
2. Fix complete-run promotion and module archive equivalence. Prove both locally against failure fixtures and an existing published version before another tag.
3. Correct the cache actions and restore the full box-1.4 runbook requirements.
4. Run the corrected candidate through literal onboarding, project-lock selection, generated CI, and fresh-pod/private-media checks. Attach evidence to the intent.
5. Complete box 5.1, reconcile roadmap and plan references, then rerun order 8 on the exact accepted artifacts.

The five earlier findings received meaningful fixes. The remaining findings are specific and testable; address them without expanding the alpha's feature scope.
