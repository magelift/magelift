# Acceptance command dependencies

Every top-level `scripts/*-acceptance-local.sh` entrypoint performs its
dependency check before it invokes a provider command. The check is shared by
`scripts/acceptance/lib-dependencies.sh`, so the AWS, GCP, Scaleway, OVHcloud,
Fastly, and New Relic harnesses have the same failure behavior. The OVHcloud
and Scaleway Kubernetes aliases immediately delegate to the checked
`scripts/k8s-acceptance-local.sh` harness.

The required JSON/YAML tools are:

| Tool | Requirement | Why |
|------|-------------|-----|
| `jq` | A working `jq` executable with `--arg`/filter support | Parses provider/API responses, checkpoints, ownership inventories, and evidence. |
| `yq` | Mike Farah `yq` v4 with `strenv` and in-place file mutation | Reads and patches YAML acceptance configuration. The harness rejects other `yq` implementations, v3 syntax, and executables that cannot safely evaluate and update a YAML file. |

Entry points also check their other local executables before mutation. For
example, the Cloud SQL cell checks `gcloud`, `curl`, `go`, and `shasum`, while
the Pub/Sub cell checks `gcloud` and `go`. The shared
`acceptance_require_commands` helper reports every missing command in one
preflight, so users can install the complete set before paying for a retry.
The live entrypoint coverage is:

| Entrypoint | Additional preflighted commands |
|------------|----------------------------------|
| AWS matrix | `aws`, `pulumi`, `docker` |
| AWS CloudWatch | `aws`, `go`, `shasum` |
| AWS SQS | `aws`, `go` |
| AWS S3 recovery | `aws`, `go` |
| AWS Secrets Manager recovery | `aws`, `go` |
| Fastly | `fastly`, `cf` |
| GCP matrix | `gcloud`, `docker`, `pulumi`, `go`, `kubectl`, `curl`, `openssl`, `shasum` |
| GCP Cloud SQL | `gcloud`, `curl`, `go`, `shasum` |
| GCP Observability | `gcloud`, `go`, `shasum`, `curl` |
| GCP Cloud Storage recovery | `gcloud`, `go` |
| GCP Secret Manager recovery | `gcloud`, `go` |
| GCP Pub/Sub | `gcloud`, `go` |
| OVHcloud/Scaleway Kubernetes | `docker`, `pulumi`, `openssl`, plus the selected provider CLI; OVH also checks `curl` and `shasum` |
| OVHcloud Object Storage recovery | `ovhcloud`, `go` |
| New Relic event probe | `newrelic` |
| New Relic operations probe | `newrelic`, `go` |
| New Relic OTLP probe | `newrelic`, `go` |
| Scaleway Object Storage recovery | `scw`, `go` |
| Scaleway Secret Manager recovery | `scw`, `go` |
| Scaleway Cockpit | `scw`, `go`, `shasum` |

Each wrapper requests the JSON/YAML checks it needs; wrappers that invoke `yq`
use the combined check and therefore require Mike Farah yq v4. The JSON/YAML
checks and each command group are aggregated, so a workstation missing several
tools receives all applicable diagnostics in the same run.

The scripts do not install software automatically. Automatic installation
would mutate a developer's machine during a paid certification and would be
ambiguous on systems with multiple package managers. Instead, a missing or
incompatible dependency stops the run before provider mutation and prints an
install hint:

```text
acceptance dependency jq is required before provisioning; install it with: brew install jq
```

The accepted tool contracts follow the upstream projects: [jq installation
guidance](https://github.com/jqlang/jq/wiki/Installation) and [Mike Farah yq
installation guidance](https://github.com/mikefarah/yq#install). In particular,
the generic `yq` package name is not sufficient: the harness requires the
Mike Farah v4 dialect because other tools use the same executable name.

On Linux, the helper prefers the detected package manager for `jq`. It always
points users to the official Mike Farah release for `yq` when a distribution
package might provide a different tool. On other systems it prints the
official installation-documentation path. CI should install and pin both
tools explicitly rather than relying on a developer workstation.

The GitHub Actions `acceptance-contracts` job installs `jq` from the runner's
package repository and the Mike Farah `yq` Linux release at a pinned version
and SHA-256, then runs `make acceptance-harness-test`. It does not authenticate
to a provider or provision cloud resources.

To verify the local setup without cloud credentials or provider mutation:

```sh
make acceptance-dependencies-check
```

`make acceptance-harness-test` declares the dependency check as a make
prerequisite, so it runs before any offline contract or dry-run command. The
offline contract tests therefore fail with the same actionable message instead
of surfacing a later `command not found` from a JSON assertion. The harnesses
do not silently install missing software; that avoids changing a developer's
machine during a paid run and keeps package-manager choice explicit.

Dry-run is not a way to bypass the dependency contract. A wrapper configured
with `MAGELIFT_ACCEPTANCE_DRY_RUN=1` still requires its applicable jq/yq
contract and wrapper-specific local commands before entering the no-mutation
path. This keeps dry-run useful for fixture and shell-contract validation while
preventing an environment that cannot execute the live JSON/YAML path from
being treated as certified. Some wrappers may still perform read-only identity
or project lookups before reporting dry-run success; they must not create,
update, or delete provider resources.

The GCP matrix also performs a local filesystem admission before ADC, API
enablement, state-backend creation, or the provider-specific Go build. It
requires 12 GiB free by default because the serial Go/Pulumi build can use
temporary space well beyond the final binary; set
`MAGELIFT_GCP_ACCEPTANCE_MIN_FREE_MB` only after measuring a smaller, stable
build footprint.

The OVHcloud Object Storage recovery cell also requires an explicit project
scope before live mutation:

```sh
MAGELIFT_OVH_RECOVERY_PROJECT=<exact-project-id> make ovh-recovery-acceptance-local
```

With no `MAGELIFT_OVH_S3_ACCESS_KEY` and
`MAGELIFT_OVH_S3_SECRET_KEY`, that cell creates one uniquely described
temporary `objectstore_operator` user through the authenticated `ovhcloud`
profile, uses its S3 credential only in the child-process environment, and
revokes/deletes the credential and user after the generated bucket is empty.
Caller-owned credentials require `MAGELIFT_OVH_S3_OWNER_ID` so the wrapper can
create the bucket under the exact S3 owner without guessing across identities.

The test enumerates every top-level acceptance wrapper, checks that direct
wrappers source and run the shared preflight before provider commands and
dry-run branches, verifies that the yq probe can perform the same `strenv`-
based in-place update used by the live matrix, and rejects jq-style `--arg` or
`--argjson` flags with `yq` v4.
