# Getting started

Path for an agency tech lead or SME Magento owner: install the CLI, validate a
sample config, then either stay local or create a disposable AWS preview.

**Target:** under 30 minutes from clone → validated `magelift.yaml` (local path),
or under ~one hour to a cloud preview URL after bootstrap (AWS).

## 1. Install

Prefer a GitHub Release archive (checksums + SBOM). Install notes live in the
repository README. From source on a small machine:

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go install github.com/acourtiol/magelift/cmd/magelift@latest
magelift version
```

## 2. Local vs cloud

Read [local vs cloud](local-vs-cloud.md). Local Compose is for Magento lifecycle
without cloud credentials; it is not a production replica.

```sh
magelift dev init
magelift dev up
```

## 3. Sample shop config

Copy [examples/sample-shop/magelift.yaml](../examples/sample-shop/magelift.yaml)
into your Magento repo root. Replace accounts, domains, and secret ARNs.

```sh
magelift doctor
magelift config validate --env preview
```

## 4. AWS preview (certified)

Requires AWS credentials and an access-log bucket.

1. `magelift bootstrap --env preview --access-log-bucket … --github-owner … --github-repo …`
2. Build or promote a signed image digest into config
3. `magelift preview --env preview` then `magelift deploy --env preview --yes`
4. `magelift outputs` / `magelift health`
5. Destroy when done: `magelift destroy --env preview --yes`

GCP GKE Autopilot is also **certified** — see [gcp-acceptance.md](gcp-acceptance.md)
for the operator harness (your disposable project via `MAGELIFT_GCP_PROJECT`).

## 5. Leaving PaaS

Config import + dump + DNS cutover:
[migrating from PaaS](migrating-from-paas.md) and the weekend packaging guide
[leave PaaS in a weekend](weekend-migrate.md).

## Decision pages

| Question | Doc |
| --- | --- |
| What is production-supported? | [Capability matrix](capability-matrix.md) |
| Can we cut a public tag? | [Release readiness](release-readiness.md) |
| Why not stay on ACC/Upsun? | [Compare to PaaS](compare-paas.md) |
| Architecture boundaries | [Architecture](architecture.md), [ADRs](adr/README.md) |

Independent of Adobe Inc. Product names are descriptive only.
