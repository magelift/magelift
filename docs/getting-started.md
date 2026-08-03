# Getting started (AWS preview)

Deploy Magento in your own AWS account with ACC/Upsun-shaped YAML — without
renting a PaaS. This path targets a careful operator and a disposable preview
environment. Production HA takes longer; treat it as a separate week.

## Before you start

- Magento Open Source repo with Composer dependencies installable
- AWS account + credentials (`aws sts get-caller-identity`)
- An S3 bucket for ALB/CloudFront access logs in the target region
- Secrets Manager entries for Composer (if private packages) and Magento crypt key

## Hour-shaped path

1. Copy `examples/sample-shop/magelift.yaml` from the repo into your Magento
   repository root and replace accounts, domains, and secret ARNs.
2. `magelift doctor`
3. `magelift bootstrap --env preview …` (once per account/region)
4. `magelift config validate --env preview`
5. Build or promote a signed image digest into config
6. `magelift preview --env preview` then `magelift deploy --env preview --yes`
7. `magelift outputs` / `magelift health`

Destroy when done: `magelift destroy --env preview --yes`.

Measure a maintainer run with `scripts/time-to-preview.sh`. Publish the elapsed
seconds in [release readiness](release-readiness.md) only after a real success.

## Queue cost note

Preview uses Magento database queues by default. For staging/production, prefer
`catalog.queueMode: ecs-rabbitmq` unless you explicitly want Amazon MQ. See the
[capability matrix](capability-matrix.md).

## Local first

If you are not ready for AWS yet:

```sh
magelift dev init
magelift dev up
```

Read [local vs cloud](local-vs-cloud.md) so Compose surprises do not look like
production bugs.
